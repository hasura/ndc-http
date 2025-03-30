package ndc

import (
	"fmt"

	"github.com/hasura/ndc-http/ndc-http-schema/ndc/internal"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// BuildTransformResponseSchema applies and builds the new schema with response transform settings.
func BuildTransformResponseSchema(ndcSchema *rest.NDCHttpSchema) (*rest.NDCHttpSchema, error) {
	if len(ndcSchema.Settings.ResponseTransforms) == 0 {
		return ndcSchema, nil
	}

	for i, rt := range ndcSchema.Settings.ResponseTransforms {
		ndcSchema, targets, err := internal.NewResponseTransformer(ndcSchema, rt).Transform()
		if err != nil {
			return nil, fmt.Errorf("%d: %w", i, err)
		}

		rt.Targets = targets
		ndcSchema.Settings.ResponseTransforms[i] = rt
	}

	return ndcSchema, nil
}
