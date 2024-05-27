package internal

import (
	"encoding/json"
	"fmt"
	"reflect"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

const (
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

const (
	RESTOptionsArgumentName          string = "restOptions"
	RESTSingleOptionsObjectName      string = "RestSingleOptions"
	RESTDistributedOptionsObjectName string = "RestDistributedOptions"
	RESTServerIDScalarName           string = "RestServerId"
	DistributedErrorObjectName       string = "DistributedError"
)

// RESTOptions represent execution options for REST requests
type RESTOptions struct {
	Servers  []string `json:"servers" yaml:"serverIds"`
	Parallel bool     `json:"parallel" yaml:"parallel"`

	Distributed bool                  `json:"-" yaml:"-"`
	Settings    *rest.NDCRestSettings `json:"-" yaml:"-"`
}

// SingleObjectType returns the object type of REST execution options for single server
func (ro RESTOptions) SingleObjectType() *schema.ObjectType {
	return &schema.ObjectType{
		Description: utils.ToPtr("Execution options for REST requests to a single server"),
		Fields: schema.ObjectTypeFields{
			"servers": schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request. If there are many server IDs the server is selected randomly"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(RESTServerIDScalarName))).Encode(),
			},
		},
	}
}

// DistributedObjectType returns the object type of REST execution options for distributed servers
func (ro RESTOptions) DistributedObjectType() *schema.ObjectType {
	return &schema.ObjectType{
		Description: utils.ToPtr("Distributed execution options for REST requests to multiple servers"),
		Fields: schema.ObjectTypeFields{
			"servers": schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(RESTServerIDScalarName))).Encode(),
			},
			"parallel": schema.ObjectField{
				Description: utils.ToPtr("Execute requests to remote servers in parallel"),
				Type:        schema.NewNullableNamedType(string(rest.ScalarBoolean)).Encode(),
			},
		},
	}
}

// FromValue parses rest execution options from any value
func (ro *RESTOptions) FromValue(value any) error {
	if utils.IsNil(value) {
		return nil
	}
	valueMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid rest options; expected object, got %v", value)
	}
	rawServerIds, ok := valueMap["servers"]
	if ok && !utils.IsNil(rawServerIds) {
		serverIDs, ok := rawServerIds.([]any)
		if !ok {
			return fmt.Errorf("invalid servers in rest options; expected []string, got %v", reflect.TypeOf(rawServerIds).Kind())
		}
		for _, v := range serverIDs {
			ro.Servers = append(ro.Servers, fmt.Sprint(v))
		}
	}

	parallel, err := utils.GetNullableBool(valueMap, "parallel")
	if err != nil {
		return fmt.Errorf("invalid parallel in rest options: %s", err)
	}
	ro.Parallel = parallel != nil && *parallel

	return nil
}

// DistributedError represents the error response of the remote request
type DistributedError struct {
	schema.ConnectorError

	// Identity of the remote server
	Server string `json:"server" yaml:"server"`
}

// Error implements the Error interface
func (de DistributedError) Error() string {
	if de.Message != "" {
		return fmt.Sprintf("%s: %s", de.Server, de.Message)
	}
	bs, _ := json.Marshal(de.Details)
	return fmt.Sprintf("%s: %s", de.Server, string(bs))
}

// DistributedResult contains the success response of remote requests with a server identity
type DistributedResult[T any] struct {
	Server string `json:"server" yaml:"server"`
	Data   T      `json:"data" yaml:"data"`
}

// DistributedResponse represents the response object of distributed operations
type DistributedResponse[T any] struct {
	Results []DistributedResult[T] `json:"results" yaml:"results"`
	Errors  []DistributedError     `json:"errors" yaml:"errors"`
}

// NewDistributedResponse creates an empty DistributedResponse instance
func NewDistributedResponse[T any]() *DistributedResponse[T] {
	return &DistributedResponse[T]{
		Results: []DistributedResult[T]{},
		Errors:  []DistributedError{},
	}
}
