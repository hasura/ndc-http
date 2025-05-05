package configuration

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/hasura/ndc-http/exhttp"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"gopkg.in/yaml.v3"
)

// ConfigValidator manages the validation and status of upstreams.
type ConfigValidator struct {
	config       *Configuration
	templates    *template.Template
	mergedSchema *schema.NDCHttpSchema
	logger       *slog.Logger
	contextPath  string
	noColor      bool

	requiredVariables         map[string]bool
	requiredHeadersForwarding map[schema.SecuritySchemeType]bool
	warnings                  map[string][]string
	errors                    map[string][]string
}

// ValidateConfiguration evaluates, validates the configuration and suggests required actions to make the connector working.
func ValidateConfiguration(
	config *Configuration,
	contextPath string,
	schemas []NDCHttpRuntimeSchema,
	mergedSchema *schema.NDCHttpSchema,
	logger *slog.Logger,
	noColor bool,
) (*ConfigValidator, error) {
	templates, err := getTemplates()
	if err != nil {
		return nil, err
	}

	cv := &ConfigValidator{
		config:                    config,
		logger:                    logger,
		templates:                 templates,
		noColor:                   noColor,
		mergedSchema:              mergedSchema,
		requiredVariables:         make(map[string]bool),
		requiredHeadersForwarding: map[schema.SecuritySchemeType]bool{},
		contextPath:               contextPath,
		errors:                    map[string][]string{},
		warnings:                  map[string][]string{},
	}

	for _, item := range schemas {
		if err := cv.evaluateSchema(&item); err != nil {
			return cv, err
		}
	}

	return cv, nil
}

// IsOk checks if the configuration has nothing to be complained.
func (cv *ConfigValidator) IsOk() bool {
	return len(cv.requiredHeadersForwarding) == 0 &&
		len(cv.requiredVariables) == 0 &&
		len(cv.warnings) == 0 &&
		len(cv.errors) == 0
}

// HasError checks if the configuration has error.
func (cv *ConfigValidator) HasError() bool {
	return len(cv.errors) > 0
}

// Render renders the help text.
func (cv *ConfigValidator) Render(w io.Writer) {
	if len(cv.errors) > 0 {
		writeErrorIf(w, ":", cv.noColor)

		for ns, errs := range cv.errors {
			_, _ = w.Write([]byte("\n\n"))
			_, _ = w.Write([]byte(ns))

			for _, err := range errs {
				_, _ = w.Write([]byte("\n  * "))
				_, _ = w.Write([]byte(err))
			}
		}
	}

	if len(cv.warnings) > 0 || len(cv.requiredHeadersForwarding) > 0 {
		writeWarningIf(w, ":\n", cv.noColor)

		if len(cv.requiredHeadersForwarding) > 0 &&
			(!cv.config.ForwardHeaders.Enabled || cv.config.ForwardHeaders.ArgumentField == nil || *cv.config.ForwardHeaders.ArgumentField == "") {
			_, _ = fmt.Fprintf(
				w,
				"\n  * Authorization header must be forwarded for the following authentication schemes: %v",
				utils.GetSortedKeys(cv.requiredHeadersForwarding),
			)
			_, _ = w.Write(
				[]byte(
					"\n    See https://github.com/hasura/ndc-http/blob/main/docs/authentication.md#headers-forwarding for more information.",
				),
			)
		}

		for ns, errs := range cv.warnings {
			_, _ = w.Write([]byte("\n\n  "))
			_, _ = w.Write([]byte(ns))
			_, _ = w.Write([]byte("\n"))

			for _, err := range errs {
				_, _ = w.Write([]byte("\n    * "))
				_, _ = w.Write([]byte(err))
			}
		}
	}

	if len(cv.requiredVariables) > 0 {
		writeColorTextIf(w, "\n\nEnvironment Variables:\n", ansiBrightYellow, cv.noColor)

		var prefix string

		serviceName := cv.getServiceName()
		if serviceName != "" {
			prefix = strings.ToUpper(serviceName) + "_"
		}

		variables := make([][]string, 0, len(cv.requiredVariables))
		for _, key := range utils.GetSortedKeys(cv.requiredVariables) {
			variables = append(variables, []string{key, prefix + key})
		}

		err := cv.templates.ExecuteTemplate(w, templateEnvVariables, map[string]any{
			"ContextPath": cv.contextPath,
			"ServiceName": serviceName,
			"Variables":   variables,
		})
		if err != nil {
			slog.Error(fmt.Sprintf("failed to render environment variables: %s", err))
		}
	}
}

func (cv *ConfigValidator) getServiceName() string {
	subgraphName := cv.findSubgraphName()
	if subgraphName == "" {
		return ""
	}

	connectorName := cv.findConnectorName()
	if connectorName == "" {
		return ""
	}

	return subgraphName + "_" + connectorName
}

func (cv *ConfigValidator) evaluateSchema(ndcSchema *NDCHttpRuntimeSchema) error {
	if ndcSchema.Settings == nil || len(ndcSchema.Settings.Servers) == 0 {
		errorMsg, err := cv.renderTemplate(templateEmptySettings, map[string]any{
			"ContextPath": cv.contextPath,
			"Namespace":   ndcSchema.Name,
		})
		if err != nil {
			return err
		}

		cv.addError(ndcSchema.Name, errorMsg)

		return nil
	}

	for _, header := range ndcSchema.Settings.Headers {
		_, err := header.Get()
		if err != nil && header.Variable != nil {
			cv.requiredVariables[*header.Variable] = true
		}
	}

	for key, ss := range ndcSchema.Settings.SecuritySchemes {
		schemeKey := "settings.securitySchemes." + key
		cv.validateSecurityScheme(ndcSchema.Name, schemeKey, ss)
	}

	if ndcSchema.Settings.TLS != nil {
		cv.validateTLS(ndcSchema.Name, "settings.tls", ndcSchema.Settings.TLS)
	}

	cv.validateArgumentPresets(
		ndcSchema.Name,
		"settings.argumentPresets",
		ndcSchema.Settings.ArgumentPresets,
		true,
	)

	for i, server := range ndcSchema.Settings.Servers {
		serverPath := fmt.Sprintf("settings.server[%d]", i)

		if server.URL.Value == nil {
			_, err := server.URL.Get()
			if err == nil {
				continue
			}

			if server.URL.Variable != nil {
				cv.requiredVariables[*server.URL.Variable] = true
			} else {
				cv.addError(ndcSchema.Name, fmt.Sprintf("%s: %s", serverPath, err))
			}
		}

		for _, header := range server.Headers {
			_, err := header.Get()
			if err != nil && header.Variable != nil {
				cv.requiredVariables[*header.Variable] = true
			}
		}

		for key, ss := range server.SecuritySchemes {
			schemeKey := fmt.Sprintf("%s.securitySchemes.%s", serverPath, key)
			cv.validateSecurityScheme(ndcSchema.Name, schemeKey, ss)
		}

		if server.TLS != nil {
			cv.validateTLS(ndcSchema.Name, serverPath+".tls", server.TLS)
		}

		cv.validateArgumentPresets(
			ndcSchema.Name,
			serverPath+".argumentPresets",
			server.ArgumentPresets,
			false,
		)
	}

	return nil
}

func (cv *ConfigValidator) validateArgumentPresets(
	namespace string,
	key string,
	argumentPresets []schema.ArgumentPresetConfig,
	isGlobal bool,
) {
	for i, preset := range argumentPresets {
		_, _, err := ValidateArgumentPreset(cv.mergedSchema, preset, isGlobal)
		if err != nil {
			cv.addError(namespace, fmt.Sprintf("%s[%d]: %s", key, i, err))

			continue
		}

		switch t := preset.Value.Interface().(type) {
		case *schema.ArgumentPresetValueEnv:
			if _, envOk := os.LookupEnv(t.Name); !envOk {
				cv.requiredVariables[t.Name] = true
			}
		case *schema.ArgumentPresetValueForwardHeader:
			cv.addWarning(namespace, fmt.Sprintf("Make sure that the %s header is added to the header forwarding list.", t.Name))
		}
	}
}

func (cv *ConfigValidator) validateTLS(namespace string, key string, tlsConfig *exhttp.TLSConfig) {
	cv.validateTLSCert(tlsConfig)
	cv.validateTLSCA(tlsConfig)
	cv.validateTLSKey(tlsConfig)
	cv.validateInsecureSkipVerify(namespace, key, tlsConfig)
}

func (cv *ConfigValidator) validateTLSCert(tlsConfig *exhttp.TLSConfig) {
	if tlsConfig.CertPem == nil && tlsConfig.CertFile == nil {
		return
	}

	if tlsConfig.CertPem != nil {
		_, err := tlsConfig.CertPem.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.CertFile != nil {
		_, err := tlsConfig.CertFile.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.CertPem != nil && tlsConfig.CertPem.Variable != nil {
		cv.requiredVariables[*tlsConfig.CertPem.Variable] = true
	} else if tlsConfig.CertFile != nil && tlsConfig.CertFile.Variable != nil {
		cv.requiredVariables[*tlsConfig.CertFile.Variable] = true
	}
}

func (cv *ConfigValidator) validateTLSCA(tlsConfig *exhttp.TLSConfig) {
	if tlsConfig.CAPem == nil && tlsConfig.CAFile == nil {
		return
	}

	if tlsConfig.CAPem != nil {
		_, err := tlsConfig.CAPem.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.CAFile != nil {
		_, err := tlsConfig.CAFile.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.CAPem != nil && tlsConfig.CAPem.Variable != nil {
		cv.requiredVariables[*tlsConfig.CAPem.Variable] = true
	} else if tlsConfig.CAFile != nil && tlsConfig.CAFile.Variable != nil {
		cv.requiredVariables[*tlsConfig.CAFile.Variable] = true
	}
}

func (cv *ConfigValidator) validateTLSKey(tlsConfig *exhttp.TLSConfig) {
	if tlsConfig.KeyPem == nil && tlsConfig.KeyFile == nil {
		return
	}

	if tlsConfig.KeyPem != nil {
		_, err := tlsConfig.KeyPem.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.KeyFile != nil {
		_, err := tlsConfig.KeyFile.Get()
		if err == nil {
			return
		}
	}

	if tlsConfig.KeyPem != nil && tlsConfig.KeyPem.Variable != nil {
		cv.requiredVariables[*tlsConfig.KeyPem.Variable] = true
	} else if tlsConfig.KeyFile != nil && tlsConfig.KeyFile.Variable != nil {
		cv.requiredVariables[*tlsConfig.KeyFile.Variable] = true
	}
}

func (cv *ConfigValidator) validateInsecureSkipVerify(
	namespace string,
	key string,
	tlsConfig *exhttp.TLSConfig,
) {
	if tlsConfig.InsecureSkipVerify == nil {
		return
	}

	_, err := tlsConfig.InsecureSkipVerify.Get()
	if err == nil {
		return
	}

	if tlsConfig.InsecureSkipVerify.Variable != nil {
		cv.requiredVariables[*tlsConfig.InsecureSkipVerify.Variable] = true
	} else {
		cv.addError(namespace, fmt.Sprintf("%s: %s", key, err))
	}
}

func (cv *ConfigValidator) validateSecurityScheme(
	namespace string,
	key string,
	ss schema.SecurityScheme,
) {
	if err := ss.Validate(); err != nil {
		cv.addError(namespace, fmt.Sprintf("%s: %s", key, err))

		return
	}

	switch schemer := ss.SecuritySchemer.(type) {
	case *schema.APIKeyAuthConfig:
		_, err := schemer.Value.Get()
		if err != nil && schemer.Value.Variable != nil {
			cv.requiredVariables[*schemer.Value.Variable] = true
		}
	case *schema.HTTPAuthConfig:
		_, err := schemer.Value.Get()
		if err != nil && schemer.Value.Variable != nil {
			cv.requiredVariables[*schemer.Value.Variable] = true
		}
	case *schema.BasicAuthConfig:
		_, err := schemer.Username.Get()
		if err != nil && schemer.Username.Variable != nil {
			cv.requiredVariables[*schemer.Username.Variable] = true
		}

		_, err = schemer.Password.Get()
		if err != nil && schemer.Password.Variable != nil {
			cv.requiredVariables[*schemer.Password.Variable] = true
		}
	case *schema.MutualTLSAuthConfig:
	case *schema.OAuth2Config:
		cv.validateOAuth2Config(namespace, key, schemer)
	default:
		cv.requiredHeadersForwarding[schemer.GetType()] = true
	}
}

func (cv *ConfigValidator) validateOAuth2Config(
	namespace string,
	key string,
	schemer *schema.OAuth2Config,
) {
	for flowType, flow := range schemer.Flows {
		if flowType != schema.ClientCredentialsFlow {
			cv.requiredHeadersForwarding[schemer.GetType()] = true

			continue
		}

		defaultMessage := ""
		if cv.config != nil {
			defaultMessage = ". You should add configuration for OAuth2 security scheme or enable header forwarding"
		}

		if flow.TokenURL == nil {
			cv.addWarning(namespace, fmt.Sprintf("%s.flow.tokenUrl is null%s", key, defaultMessage))
		} else {
			_, err := flow.TokenURL.Get()
			if err != nil && flow.TokenURL.Variable != nil {
				cv.requiredVariables[*flow.TokenURL.Variable] = true
			}
		}

		if flow.ClientID == nil {
			cv.addWarning(namespace, fmt.Sprintf("%s.flow.clientId is null%s", key, defaultMessage))
		} else {
			_, err := flow.ClientID.Get()
			if err != nil && flow.ClientID.Variable != nil {
				cv.requiredVariables[*flow.ClientID.Variable] = true
			}
		}

		if flow.ClientSecret == nil {
			cv.addWarning(
				namespace,
				fmt.Sprintf("%s.flow.clientSecret is null%s", key, defaultMessage),
			)
		} else {
			_, err := flow.ClientSecret.Get()
			if err != nil && flow.ClientSecret.Variable != nil {
				cv.requiredVariables[*flow.ClientSecret.Variable] = true
			}
		}

		for _, param := range flow.EndpointParams {
			_, err := param.Get()
			if err != nil && param.Variable != nil {
				cv.requiredVariables[*param.Variable] = true
			}
		}
	}
}

type manifestDefinition struct {
	Definition struct {
		Name string `yaml:"name"`
	} `yaml:"definition"`
}

func (cv *ConfigValidator) findConnectorName() string {
	if cv.contextPath == "" || cv.contextPath == "." {
		return ""
	}

	connectorPath := filepath.Join(cv.contextPath, "connector.yaml")

	rawBytes, err := os.ReadFile(connectorPath)
	if err != nil {
		cv.logger.Error(fmt.Sprintf("failed to read the connector manifest: %s", err))

		return ""
	}

	var definition manifestDefinition
	if err := yaml.Unmarshal(rawBytes, &definition); err != nil {
		cv.logger.Error(fmt.Sprintf("failed to decode the connector manifest: %s", err))

		return ""
	}

	return definition.Definition.Name
}

func (cv *ConfigValidator) findSubgraphName() string {
	if cv.contextPath == "" || cv.contextPath == "." {
		return ""
	}

	connectorPath := filepath.Join(cv.contextPath, "..", "..", "subgraph.yaml")

	rawBytes, err := os.ReadFile(connectorPath)
	if err != nil {
		cv.logger.Error(fmt.Sprintf("failed to read the subgraph manifest: %s", err))

		return ""
	}

	var definition manifestDefinition
	if err := yaml.Unmarshal(rawBytes, &definition); err != nil {
		cv.logger.Error(fmt.Sprintf("failed to decode the subgraph manifest: %s", err))

		return ""
	}

	return definition.Definition.Name
}

func (cv *ConfigValidator) renderTemplate(name string, data map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := cv.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (cv *ConfigValidator) addWarning(namespace string, value string) {
	_, ok := cv.warnings[namespace]
	if !ok {
		cv.warnings[namespace] = []string{value}
	} else {
		cv.warnings[namespace] = append(cv.warnings[namespace], value)
	}
}

func (cv *ConfigValidator) addError(namespace string, value string) {
	_, ok := cv.errors[namespace]
	if !ok {
		cv.errors[namespace] = []string{value}
	} else {
		cv.errors[namespace] = append(cv.errors[namespace], value)
	}
}
