package openapi

import (
	"encoding/json"

	"github.com/hasura/ndc-http/ndc-http-schema/openapi/internal"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// BuildNDCSchema validates and builds the NDC schema.
func BuildNDCSchema(input []byte, options ConvertOptions) (*rest.NDCHttpSchema, error) {
	var result *rest.NDCHttpSchema

	if err := json.Unmarshal(input, &result); err != nil {
		return nil, err
	}

	return internal.NewNDCBuilder(result, internal.ConvertOptions(options)).Build()
}
