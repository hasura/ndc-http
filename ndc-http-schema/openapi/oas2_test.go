package openapi

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestOpenAPIv2ToRESTSchema(t *testing.T) {
	testCases := []struct {
		Name     string
		Source   string
		Options  ConvertOptions
		Expected string
		Schema   string
	}{
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/jsonplaceholder/swagger.json -o ./ndc-http-schema/openapi/testdata/jsonplaceholder/expected.json --spec oas2 --trim-prefix /v1
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/jsonplaceholder/swagger.json -o ./ndc-http-schema/openapi/testdata/jsonplaceholder/schema.json --pure --spec oas2 --trim-prefix /v1
		{
			Name:     "jsonplaceholder",
			Source:   "testdata/jsonplaceholder/swagger.json",
			Expected: "testdata/jsonplaceholder/expected.json",
			Schema:   "testdata/jsonplaceholder/schema.json",
			Options: ConvertOptions{
				TrimPrefix: "/v1",
			},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/petstore2/swagger.json -o ./ndc-http-schema/openapi/testdata/petstore2/expected.json --spec oas2
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/petstore2/swagger.json -o ./ndc-http-schema/openapi/testdata/petstore2/schema.json --pure --spec oas2
		{
			Name:     "petstore2",
			Source:   "testdata/petstore2/swagger.json",
			Expected: "testdata/petstore2/expected.json",
			Schema:   "testdata/petstore2/schema.json",
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix2/source.json -o ./ndc-http-schema/openapi/testdata/prefix2/expected_single_word.json --spec oas2 --prefix hasura
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix2/source.json -o ./ndc-http-schema/openapi/testdata/prefix2/expected_single_word.schema.json --pure --spec oas2 --prefix hasura
		{
			Name:     "prefix2_single_word",
			Source:   "testdata/prefix2/source.json",
			Expected: "testdata/prefix2/expected_single_word.json",
			Schema:   "testdata/prefix2/expected_single_word.schema.json",
			Options: ConvertOptions{
				Prefix: "hasura",
			},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix2/source.json -o ./ndc-http-schema/openapi/testdata/prefix2/expected_multi_words.json --spec oas2 --prefix hasura_mock_json
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix2/source.json -o ./ndc-http-schema/openapi/testdata/prefix2/expected_multi_words.schema.json --pure --spec oas2 --prefix hasura_mock_json
		{
			Name:     "prefix2_multi_word",
			Source:   "testdata/prefix2/source.json",
			Expected: "testdata/prefix2/expected_multi_words.json",
			Schema:   "testdata/prefix2/expected_multi_words.schema.json",
			Options: ConvertOptions{
				Prefix: "hasura_mock_json",
			},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/union2/source.json -o ./ndc-http-schema/openapi/testdata/union2/expected.json --spec oas2
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/union2/source.json -o ./ndc-http-schema/openapi/testdata/union2/schema.json --pure --spec oas2
		{
			Name:     "union2",
			Source:   "testdata/union2/source.json",
			Expected: "testdata/union2/expected.json",
			Schema:   "testdata/union2/schema.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(tc.Source)
			assert.NilError(t, err)

			expectedBytes, err := os.ReadFile(tc.Expected)
			assert.NilError(t, err)
			var expected rest.NDCHttpSchema
			assert.NilError(t, json.Unmarshal(expectedBytes, &expected))

			output, errs := OpenAPIv2ToNDCSchema(sourceBytes, tc.Options)
			if output == nil {
				t.Error(errors.Join(errs...))
				t.FailNow()
			}

			bs, err := json.Marshal(output)
			assert.NilError(t, err)

			var result rest.NDCHttpSchema
			assert.NilError(t, json.Unmarshal(bs, &result))
			assertRESTSchemaEqual(t, &expected, &result)
			assertConnectorSchema(t, tc.Schema, output)
		})
	}

	t.Run("failure_empty", func(t *testing.T) {
		_, err := OpenAPIv2ToNDCSchema([]byte(""), ConvertOptions{})
		assert.ErrorContains(t, errors.Join(err...), "there is nothing in the spec, it's empty")
	})
}

func assertConnectorSchema(t *testing.T, schemaPath string, output *rest.NDCHttpSchema) {
	t.Helper()
	if schemaPath == "" {
		return
	}
	schemaBytes, err := os.ReadFile(schemaPath)
	assert.NilError(t, err)
	var expectedSchema schema.SchemaResponse
	assert.NilError(t, json.Unmarshal(schemaBytes, &expectedSchema))
	outputSchema := output.ToSchemaResponse()
	assertDeepEqual(t, expectedSchema, *outputSchema)
}
