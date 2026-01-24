package configuration

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hasura/goenvconf"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/utils"
)

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

		serviceName := cv.getServiceName()

		variables := make([]string, 0, len(cv.requiredVariables))
		variables = append(variables, utils.GetSortedKeys(cv.requiredVariables)...)

		err := cv.templates.ExecuteTemplate(w, templateEnvVariables, map[string]any{
			"ContextPath": cv.contextPath,
			"ServiceName": strings.ToUpper(serviceName),
			"Variables":   variables,
		})
		if err != nil {
			slog.Error(fmt.Sprintf("failed to render environment variables: %s", err))
		}
	}
}

// WriteReadme writes a README.md file for available configurations and instructions.
func (cv *ConfigValidator) WriteReadme() error {
	filePath := filepath.Join(cv.contextPath, "README.md")

	w, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer func() {
		_ = w.Close()
	}()

	return cv.renderReadme(w)
}

func (cv *ConfigValidator) renderReadme(w io.Writer) error {
	forwardingHeaders := utils.GetSortedKeys(cv.forwardedHeaderNames)
	if len(cv.requiredHeadersForwarding) > 0 &&
		!cv.requiredHeadersForwarding[schema.CookieAuthScheme] &&
		!cv.forwardedHeaderNames["authorization"] &&
		!cv.forwardedHeaderNames["Authorization"] {
		forwardingHeaders = append(forwardingHeaders, "Authorization (or user-defined auth header)")
	}

	data := map[string]any{
		"ContextPath":       cv.contextPath,
		"SubgraphName":      cv.subgraphName,
		"SubgraphPath":      filepath.Dir(filepath.Dir(cv.contextPath)),
		"ConnectorName":     cv.connectorName,
		"ServiceName":       strings.ToUpper(cv.getServiceName()),
		"ForwardingHeaders": forwardingHeaders,
	}

	servers := make([]schemaDocOutput, len(cv.schemaDocs))

	for i, doc := range cv.schemaDocs {
		servers[i] = doc.Output()
	}

	data["Servers"] = servers

	return cv.templates.ExecuteTemplate(w, templateReadme, data)
}

func (cv *ConfigValidator) renderTemplate(name string, data map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := cv.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (cv *ConfigValidator) getServiceName() string {
	if cv.subgraphName == "" || cv.connectorName == "" {
		return ""
	}

	return cv.subgraphName + "_" + cv.connectorName
}

func (cv *ConfigValidator) getLastSchemaDoc() *schemaDocInfo {
	return cv.schemaDocs[len(cv.schemaDocs)-1]
}

type schemaDocInfo struct {
	Name      string
	Variables map[string]schemaDocVariableInfo
}

type schemaDocOutput struct {
	Name      string
	Variables []schemaDocVariableInfo
}

func (sdi schemaDocInfo) Output() schemaDocOutput {
	result := schemaDocOutput{
		Name:      sdi.Name,
		Variables: make([]schemaDocVariableInfo, 0, len(sdi.Variables)),
	}

	keys := utils.GetSortedKeys(sdi.Variables)

	for _, key := range keys {
		result.Variables = append(result.Variables, sdi.Variables[key])
	}

	return result
}

type schemaDocVariableInfo struct {
	Name    string
	Type    string
	Default string
}

func parseSchemaDocVariableInfo(value any) schemaDocVariableInfo {
	var defaultValue string

	switch t := value.(type) {
	case goenvconf.EnvString:
		if t.Value != nil {
			defaultValue = *t.Value
		}

		return schemaDocVariableInfo{
			Name:    *t.Variable,
			Type:    "string",
			Default: defaultValue,
		}
	case goenvconf.EnvBool:
		if t.Value != nil {
			defaultValue = strconv.FormatBool(*t.Value)
		}

		return schemaDocVariableInfo{
			Name:    *t.Variable,
			Type:    "boolean",
			Default: defaultValue,
		}
	case goenvconf.EnvFloat:
		if t.Value != nil {
			defaultValue = fmt.Sprint(*t.Value)
		}

		return schemaDocVariableInfo{
			Name:    *t.Variable,
			Type:    "float",
			Default: defaultValue,
		}
	case goenvconf.EnvInt:
		if t.Value != nil {
			defaultValue = strconv.FormatInt(*t.Value, 10)
		}

		return schemaDocVariableInfo{
			Name:    *t.Variable,
			Type:    "int",
			Default: defaultValue,
		}
	default:
		return schemaDocVariableInfo{
			Default: fmt.Sprint(value),
		}
	}
}
