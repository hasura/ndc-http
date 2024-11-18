package internal

import (
	"bytes"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestCreateXMLForm(t *testing.T) {
	testCases := []struct {
		Name string
		Body map[string]any

		Expected string
	}{
		{
			Name: "putPetXml",
			Body: map[string]any{
				"id":   int64(10),
				"name": "doggie",
				"category": map[string]any{
					"id":   int64(1),
					"name": "Dogs",
				},
				"photoUrls": []any{"string"},
				"tags": []any{
					map[string]any{
						"id":   int64(0),
						"name": "string",
					},
				},
				"status": "available",
			},
			Expected: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<pet><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls><photoUrl>string</photoUrl></photoUrls><status>available</status><tags><tag><id>0</id><name>string</name></tag></tags></pet>",
		},
	}

	ndcSchema := createMockSchema(t)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var info *rest.OperationInfo
			for key, f := range ndcSchema.Procedures {
				if key == tc.Name {
					info = &f
					break
				}
			}
			assert.Assert(t, info != nil)
			argumentInfo := info.Arguments["body"]
			result, err := NewXMLEncoder(ndcSchema).Encode(&argumentInfo, tc.Body)
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.Expected, string(result))

			dec := NewXMLDecoder(ndcSchema)
			parsedResult, err := dec.Decode(bytes.NewBuffer([]byte(tc.Expected)), info.ResultType)
			assert.NilError(t, err)

			assert.DeepEqual(t, tc.Body, parsedResult)
		})
	}
}
