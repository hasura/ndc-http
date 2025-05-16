package configuration

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

	subgraphName              string
	connectorName             string
	schemaDocs                []*schemaDocInfo
	requiredVariables         map[string]bool
	forwardedHeaderNames      map[string]bool
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
		forwardedHeaderNames:      make(map[string]bool),
		requiredHeadersForwarding: map[schema.SecuritySchemeType]bool{},
		contextPath:               contextPath,
		errors:                    map[string][]string{},
		warnings:                  map[string][]string{},
	}

	cv.subgraphName = cv.findSubgraphName()
	cv.connectorName = cv.findConnectorName()

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

func (cv *ConfigValidator) evaluateSchema(ndcSchema *NDCHttpRuntimeSchema) error {
	docInfo := &schemaDocInfo{
		Name:      ndcSchema.Name,
		Variables: make(map[string]schemaDocVariableInfo),
	}

	cv.schemaDocs = append(cv.schemaDocs, docInfo)

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
		if header.Variable != nil {
			docInfo.Variables[*header.Variable] = parseSchemaDocVariableInfo(header)
		}

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

		if server.URL.Variable != nil {
			docInfo.Variables[*server.URL.Variable] = parseSchemaDocVariableInfo(server.URL)
		}

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
			if header.Variable != nil {
				docInfo.Variables[*header.Variable] = parseSchemaDocVariableInfo(header)
			}

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
	schemaDoc := cv.getLastSchemaDoc()

	for i, preset := range argumentPresets {
		_, _, err := ValidateArgumentPreset(cv.mergedSchema, preset, isGlobal)
		if err != nil {
			cv.addError(namespace, fmt.Sprintf("%s[%d]: %s", key, i, err))

			continue
		}

		switch t := preset.Value.Interface().(type) {
		case *schema.ArgumentPresetValueEnv:
			schemaDoc.Variables[t.Name] = schemaDocVariableInfo{
				Name: t.Name,
			}

			if _, envOk := os.LookupEnv(t.Name); !envOk {
				cv.requiredVariables[t.Name] = true
			}
		case *schema.ArgumentPresetValueForwardHeader:
			cv.forwardedHeaderNames[t.Name] = true
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

	schemaDoc := cv.getLastSchemaDoc()

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.CertPem) {
		return
	}

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.CertFile) {
		return
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

	schemaDoc := cv.getLastSchemaDoc()

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.CAPem) {
		return
	}

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.CAFile) {
		return
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

	schemaDoc := cv.getLastSchemaDoc()

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.KeyPem) {
		return
	}

	if cv.validateOptionalEnvString(schemaDoc, tlsConfig.KeyFile) {
		return
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

	schemaDoc := cv.getLastSchemaDoc()

	if tlsConfig.InsecureSkipVerify.Variable != nil {
		schemaDoc.Variables[*tlsConfig.InsecureSkipVerify.Variable] = parseSchemaDocVariableInfo(
			*tlsConfig.InsecureSkipVerify,
		)
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

	schemaDoc := cv.getLastSchemaDoc()

	switch schemer := ss.SecuritySchemer.(type) {
	case *schema.APIKeyAuthConfig:
		if schemer.Value.Variable != nil {
			schemaDoc.Variables[*schemer.Value.Variable] = parseSchemaDocVariableInfo(schemer.Value)
		}

		_, err := schemer.Value.Get()
		if err != nil && schemer.Value.Variable != nil {
			cv.requiredVariables[*schemer.Value.Variable] = true
		}
	case *schema.HTTPAuthConfig:
		if schemer.Value.Variable != nil {
			schemaDoc.Variables[*schemer.Value.Variable] = parseSchemaDocVariableInfo(schemer.Value)
		}

		_, err := schemer.Value.Get()
		if err != nil && schemer.Value.Variable != nil {
			cv.requiredVariables[*schemer.Value.Variable] = true
		}
	case *schema.BasicAuthConfig:
		if schemer.Username.Variable != nil {
			schemaDoc.Variables[*schemer.Username.Variable] = parseSchemaDocVariableInfo(schemer.Username)
		}

		if schemer.Password.Variable != nil {
			schemaDoc.Variables[*schemer.Password.Variable] = parseSchemaDocVariableInfo(schemer.Password)
		}

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
	case *schema.CookieAuthConfig:
		cv.forwardedHeaderNames["Cookie"] = true
		cv.requiredHeadersForwarding[schemer.GetType()] = true
	default:
		cv.requiredHeadersForwarding[schemer.GetType()] = true
	}
}

func (cv *ConfigValidator) validateOAuth2Config(
	namespace string,
	key string,
	schemer *schema.OAuth2Config,
) {
	schemaDoc := cv.getLastSchemaDoc()

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
			if flow.TokenURL.Variable != nil {
				schemaDoc.Variables[*flow.TokenURL.Variable] = parseSchemaDocVariableInfo(*flow.TokenURL)
			}

			_, err := flow.TokenURL.Get()
			if err != nil && flow.TokenURL.Variable != nil {
				cv.requiredVariables[*flow.TokenURL.Variable] = true
			}
		}

		if flow.ClientID == nil {
			cv.addWarning(namespace, fmt.Sprintf("%s.flow.clientId is null%s", key, defaultMessage))
		} else {
			if flow.ClientID.Variable != nil {
				schemaDoc.Variables[*flow.ClientID.Variable] = parseSchemaDocVariableInfo(*flow.ClientID)
			}

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
			if flow.ClientSecret.Variable != nil {
				schemaDoc.Variables[*flow.ClientSecret.Variable] = parseSchemaDocVariableInfo(*flow.ClientSecret)
			}

			_, err := flow.ClientSecret.Get()
			if err != nil && flow.ClientSecret.Variable != nil {
				cv.requiredVariables[*flow.ClientSecret.Variable] = true
			}
		}

		for _, param := range flow.EndpointParams {
			if param.Variable != nil {
				schemaDoc.Variables[*param.Variable] = parseSchemaDocVariableInfo(param)
			}

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

func (cv *ConfigValidator) validateOptionalEnvString(
	schemaDoc *schemaDocInfo,
	value *utils.EnvString,
) bool {
	if value == nil {
		return false
	}

	if value.Variable != nil {
		schemaDoc.Variables[*value.Variable] = parseSchemaDocVariableInfo(
			*value,
		)
	}

	_, err := value.Get()

	return err == nil
}
