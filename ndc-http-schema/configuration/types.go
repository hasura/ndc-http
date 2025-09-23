package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/hasura/ndc-http/exhttp"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"github.com/hasura/ndc-sdk-go/v2/utils"
)

var (
	errFilePathRequired   = errors.New("file path is empty")
	errHTTPMethodRequired = errors.New("the HTTP method is required")
)

var fieldNameRegex = regexp.MustCompile(`^[a-zA-Z_]\w+$`)

// Configuration contains required settings for the connector.
type Configuration struct {
	Output string `json:"output,omitempty"         yaml:"output,omitempty"`
	// Require strict validation
	Strict         bool                   `json:"strict"                   yaml:"strict"`
	Runtime        RawRuntimeSettings     `json:"runtime,omitempty"        yaml:"runtime,omitempty"`
	ForwardHeaders ForwardHeadersSettings `json:"forwardHeaders,omitempty" yaml:"forwardHeaders,omitempty"`
	Concurrency    ConcurrencySettings    `json:"concurrency,omitempty"    yaml:"concurrency,omitempty"`
	Files          []ConfigItem           `json:"files"                    yaml:"files"`
}

// ConcurrencySettings represent settings for concurrent webhook executions to remote servers.
type ConcurrencySettings struct {
	// Maximum number of concurrent executions if there are many query variables.
	Query uint `json:"query"    yaml:"query"`
	// Maximum number of concurrent executions if there are many mutation operations.
	Mutation uint `json:"mutation" yaml:"mutation"`
	// Maximum number of concurrent requests to remote servers (distribution mode).
	HTTP uint `json:"http"     yaml:"http"`
}

// ForwardHeadersSettings hold settings of header forwarding from and to Hasura engine.
type ForwardHeadersSettings struct {
	// Enable headers forwarding.
	Enabled bool `json:"enabled"         yaml:"enabled"`
	// The argument field name to be added for headers forwarding.
	ArgumentField *string `json:"argumentField"   yaml:"argumentField"   jsonschema:"oneof_type=string;null,pattern=^[a-zA-Z_][a-zA-Z0-9_]+$"`
	// HTTP response headers to be forwarded from a data connector to the client.
	ResponseHeaders *ForwardResponseHeadersSettings `json:"responseHeaders" yaml:"responseHeaders" jsonschema:"nullable"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ForwardHeadersSettings) UnmarshalJSON(b []byte) error {
	type Plain ForwardHeadersSettings

	var rawResult Plain
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	if !rawResult.Enabled {
		*j = ForwardHeadersSettings(rawResult)

		return nil
	}

	if rawResult.ArgumentField != nil && !fieldNameRegex.MatchString(*rawResult.ArgumentField) {
		return fmt.Errorf(
			"invalid forwardHeaders.argumentField name format: %s",
			*rawResult.ArgumentField,
		)
	}

	if rawResult.ResponseHeaders != nil {
		if err := rawResult.ResponseHeaders.Validate(); err != nil {
			return fmt.Errorf("responseHeaders: %w", err)
		}
	}

	*j = ForwardHeadersSettings(rawResult)

	return nil
}

// ForwardResponseHeadersSettings hold settings of header forwarding from http response to Hasura engine.
type ForwardResponseHeadersSettings struct {
	// Name of the field in the NDC function/procedure's result which contains the response headers.
	HeadersField string `json:"headersField"   jsonschema:"pattern=^[a-zA-Z_][a-zA-Z0-9_]+$" yaml:"headersField"`
	// Name of the field in the NDC function/procedure's result which contains the result.
	ResultField string `json:"resultField"    jsonschema:"pattern=^[a-zA-Z_][a-zA-Z0-9_]+$" yaml:"resultField"`
	// List of actual HTTP response headers from the data connector to be set as response headers. Returns all headers if empty.
	ForwardHeaders []string `json:"forwardHeaders"                                               yaml:"forwardHeaders"`
}

// Validate checks if the setting is valid.
func (j ForwardResponseHeadersSettings) Validate() error {
	if !fieldNameRegex.MatchString(j.HeadersField) {
		return fmt.Errorf("invalid format in headersField: %s", j.HeadersField)
	}

	if !fieldNameRegex.MatchString(j.ResultField) {
		return fmt.Errorf("invalid format in resultField: %s", j.ResultField)
	}

	return nil
}

// ConfigItem extends the ConvertConfig with advanced options.
type ConfigItem struct {
	ConvertConfig `yaml:",inline"`

	// Distributed enables distributed schema
	Distributed *bool `json:"distributed,omitempty" yaml:"distributed,omitempty"`
	// configure the request timeout in seconds.
	Timeout *utils.EnvInt              `json:"timeout,omitempty"     yaml:"timeout,omitempty"     mapstructure:"timeout"`
	Retry   *exhttp.RetryPolicySetting `json:"retry,omitempty"       yaml:"retry,omitempty"       mapstructure:"retry"`
}

// IsDistributed checks if the distributed option is enabled.
func (ci ConfigItem) IsDistributed() bool {
	return ci.Distributed != nil && *ci.Distributed
}

// GetRuntimeSettings validate and get runtime settings.
func (ci ConfigItem) GetRuntimeSettings() (*rest.RuntimeSettings, error) {
	result := &rest.RuntimeSettings{}

	var errs []error

	if ci.Timeout != nil {
		timeout, err := ci.Timeout.Get()

		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("timeout: %w", err))
		case timeout < 0:
			errs = append(errs, fmt.Errorf("timeout must be positive, got: %d", timeout))
		default:
			result.Timeout = uint(timeout)
		}
	}

	if ci.Retry != nil {
		retryPolicy, err := ci.Retry.Validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("ConfigItem.retry: %w", err))
		}

		if retryPolicy.Delay > 0 && result.Timeout > 0 &&
			time.Duration(
				retryPolicy.Delay,
			)*time.Millisecond > time.Duration(
				result.Timeout,
			)*time.Second {
			errs = append(errs, errors.New("retry delay duration must be less than the timeout"))
		}

		result.Retry = *retryPolicy
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}

// ConvertConfig represents the content of convert config file.
type ConvertConfig struct {
	// File path needs to be converted
	File string `json:"file"                          jsonschema:"required"     yaml:"file"`
	// The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)
	Spec rest.SchemaSpecType `json:"spec,omitempty"                jsonschema:"default=oas3" yaml:"spec"`
	// Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser
	MethodAlias map[string]string `json:"methodAlias,omitempty"                                   yaml:"methodAlias"`
	// Add a prefix to the function and procedure names
	Prefix string `json:"prefix,omitempty"                                        yaml:"prefix"`
	// Trim the prefix in URL, e.g. /v1
	TrimPrefix string `json:"trimPrefix,omitempty"                                    yaml:"trimPrefix"`
	// The environment variable prefix for security values, e.g. PET_STORE
	EnvPrefix string `json:"envPrefix,omitempty"                                     yaml:"envPrefix"`
	// Return the pure NDC schema only
	Pure bool `json:"pure,omitempty"                                          yaml:"pure"`
	// Ignore deprecated fields.
	NoDeprecation bool `json:"noDeprecation,omitempty"                                 yaml:"noDeprecation"`
	// Patch files to be applied into the input file before converting
	PatchBefore []restUtils.PatchConfig `json:"patchBefore,omitempty"                                   yaml:"patchBefore"`
	// Patch files to be applied into the input file after converting
	PatchAfter []restUtils.PatchConfig `json:"patchAfter,omitempty"                                    yaml:"patchAfter"`
	// Allowed content types. All content types are allowed by default
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty"                           yaml:"allowedContentTypes"`
	// The location where the ndc schema file will be generated. Print to stdout if not set
	Output string `json:"output,omitempty"                                        yaml:"output,omitempty"`
}

// NDCHttpRuntimeSchema wraps NDCHttpSchema with runtime settings.
type NDCHttpRuntimeSchema struct {
	*rest.NDCHttpSchema

	Name    string               `json:"name" yaml:"name"`
	Runtime rest.RuntimeSettings `json:"-"    yaml:"-"`
}

// ConvertCommandArguments represent available command arguments for the convert command.
type ConvertCommandArguments struct {
	File                string            `help:"File path needs to be converted."                                                                                            short:"f"`
	Config              string            `help:"Path of the config file."                                                                                                    short:"c"`
	Output              string            `help:"The location where the ndc schema file will be generated. Print to stdout if not set"                                        short:"o"`
	Spec                string            `help:"The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)"`
	Format              string            `help:"The output format, is one of json, yaml. If the output is set, automatically detect the format in the output file extension"           default:"json"`
	Strict              bool              `help:"Require strict validation"                                                                                                             default:"false"`
	NoDeprecation       bool              `help:"Ignore deprecated fields"                                                                                                              default:"false"`
	Pure                bool              `help:"Return the pure NDC schema only"                                                                                                       default:"false"`
	Prefix              string            `help:"Add a prefix to the function and procedure names"`
	TrimPrefix          string            `help:"Trim the prefix in URL, e.g. /v1"`
	EnvPrefix           string            `help:"The environment variable prefix for security values, e.g. PET_STORE"`
	MethodAlias         map[string]string `help:"Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser"`
	AllowedContentTypes []string          `help:"Allowed content types. All content types are allowed by default"`
	PatchBefore         []string          `help:"Patch files to be applied into the input file before converting"`
	PatchAfter          []string          `help:"Patch files to be applied into the input file after converting"`
}

// the object type of HTTP execution options for single server.
var singleObjectType = rest.ObjectType{
	Description: utils.ToPtr("Execution options for HTTP requests to a single server"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr(
					"Specify remote servers to receive the request. If there are many server IDs the server is selected randomly",
				),
				Type: schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.HTTPServerIDScalarName))).
					Encode(),
			},
		},
	},
}

// the object type of HTTP execution options for distributed servers.
var distributedObjectType rest.ObjectType = rest.ObjectType{
	Description: utils.ToPtr("Distributed execution options for HTTP requests to multiple servers"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request"),
				Type: schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.HTTPServerIDScalarName))).
					Encode(),
			},
		},
		"parallel": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Execute requests to remote servers in parallel"),
				Type:        schema.NewNullableNamedType(string(rest.ScalarBoolean)).Encode(),
			},
		},
	},
}

var httpSingleOptionsArgument = rest.ArgumentInfo{
	ArgumentInfo: schema.ArgumentInfo{
		Description: singleObjectType.Description,
		Type:        schema.NewNullableNamedType(rest.HTTPSingleOptionsObjectName).Encode(),
	},
}

// RawRuntimeSettings hold raw runtime settings.
type RawRuntimeSettings struct {
	// Enable the sendHttpRequest operation.
	EnableRawRequest *bool `json:"enableRawRequest,omitempty" yaml:"enableRawRequest,omitempty"`
	// Treat the JSON scalar as a json string
	StringifyJSON *utils.EnvBool `json:"stringifyJson,omitempty"    yaml:"stringifyJson,omitempty"`
}

// RuntimeSettings hold optional runtime settings.
type RuntimeSettings struct {
	// Enable the sendHttpRequest operation.
	EnableRawRequest bool `json:"enableRawRequest,omitempty" yaml:"enableRawRequest,omitempty"`
	// Treat the JSON scalar as a json string
	StringifyJSON bool `json:"stringifyJson,omitempty"    yaml:"stringifyJson,omitempty"`
}

// Validate validates and returns validated settings.
func (rs RawRuntimeSettings) Validate() (*RuntimeSettings, error) {
	result := RuntimeSettings{
		EnableRawRequest: rs.EnableRawRequest == nil || *rs.EnableRawRequest,
	}

	if rs.StringifyJSON != nil {
		stringifyJson, err := rs.StringifyJSON.GetOrDefault(false)
		if err != nil {
			return nil, fmt.Errorf("stringifyJson: %w", err)
		}

		result.StringifyJSON = stringifyJson
	}

	return &result, nil
}
