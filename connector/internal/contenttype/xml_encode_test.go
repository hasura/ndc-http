package contenttype

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
		{
			Name: "putCommentXml",
			Body: map[string]any{
				"user":          "Iggy",
				"comment_count": int64(6),
				"comment": []any{
					map[string]any{
						"who":       "Iggy",
						"when":      "2021-10-15 13:28:22 UTC",
						"id":        int64(1),
						"bsrequest": int64(115),
						"xmlValue":  "This is a pretty cool request!",
					},
					map[string]any{
						"who":      "Iggy",
						"when":     "2021-10-15 13:49:39 UTC",
						"id":       int64(2),
						"project":  "home:Admin",
						"xmlValue": "This is a pretty cool project!",
					},
					map[string]any{
						"who":      "Iggy",
						"when":     "2021-10-15 13:54:38 UTC",
						"id":       int64(3),
						"project":  "home:Admin",
						"package":  "0ad",
						"xmlValue": "This is a pretty cool package!",
					},
				},
			},
			Expected: `<?xml version="1.0" encoding="UTF-8"?>
<comments comment="6" user="Iggy"><comment bsrequest="115" id="1" when="2021-10-15 13:28:22 UTC" who="Iggy">This is a pretty cool request!</comment><comment id="2" project="home:Admin" when="2021-10-15 13:49:39 UTC" who="Iggy">This is a pretty cool project!</comment><comment id="3" package="0ad" project="home:Admin" when="2021-10-15 13:54:38 UTC" who="Iggy">This is a pretty cool package!</comment></comments>`,
		},
		{
			Name: "putBookXml",
			Body: map[string]any{
				"id":     int64(0),
				"title":  "string",
				"author": "Author",
				"attr":   "foo",
			},
			Expected: `<?xml version="1.0" encoding="UTF-8"?>
<smp:book smp:attr="foo" xmlns:smp="http://example.com/schema"><author>Author</author><id>0</id><title>string</title></smp:book>`,
		},
		{
			Name: "putCommentXml",
			Body: map[string]any{
				"project": "home:Admin",
				"package": "0ad",
				"comment": []any{
					map[string]any{
						"who":      "Iggy",
						"when":     "2021-10-15 13:28:22 UTC",
						"id":       int64(1),
						"xmlValue": "This is a pretty cool comment!",
					},
				},
			},
			Expected: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<comments package=\"0ad\" project=\"home:Admin\"><comment id=\"1\" when=\"2021-10-15 13:28:22 UTC\" who=\"Iggy\">This is a pretty cool comment!</comment></comments>",
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
			assert.Equal(t, tc.Expected, string(result))

			dec := NewXMLDecoder(ndcSchema)
			parsedResult, err := dec.Decode(bytes.NewBuffer([]byte(tc.Expected)), info.ResultType)
			assert.NilError(t, err)

			assert.DeepEqual(t, tc.Body, parsedResult)
		})
	}
}

func TestCreateArbitraryXMLForm(t *testing.T) {
	testCases := []struct {
		Name string
		Body map[string]any

		Expected string
	}{
		{
			Name: "putPetXml",
			Body: map[string]any{
				"id":   "10",
				"name": "doggie",
				"category": map[string]any{
					"id":   "1",
					"name": "Dogs",
				},
				"photoUrls": "string",
				"tags": map[string]any{
					"id":   "0",
					"name": "string",
				},
				"status": "available",
			},
			Expected: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<xml><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls>string</photoUrls><status>available</status><tags><id>0</id><name>string</name></tags></xml>",
		},
		{
			Name: "putCommentXml",
			Body: map[string]any{
				"user":          "Iggy",
				"comment_count": "6",
				"comment": []any{
					map[string]any{
						"who":       "Iggy",
						"when":      "2021-10-15 13:28:22 UTC",
						"id":        "1",
						"bsrequest": "115",
					},
					map[string]any{
						"who":     "Iggy",
						"when":    "2021-10-15 13:49:39 UTC",
						"id":      "2",
						"project": "home:Admin",
					},
					map[string]any{
						"who":     "Iggy",
						"when":    "2021-10-15 13:54:38 UTC",
						"id":      "3",
						"project": "home:Admin",
						"package": "0ad",
					},
				},
			},
			Expected: `<?xml version="1.0" encoding="UTF-8"?>
<xml><comment><bsrequest>115</bsrequest><id>1</id><when>2021-10-15 13:28:22 UTC</when><who>Iggy</who></comment><comment><id>2</id><project>home:Admin</project><when>2021-10-15 13:49:39 UTC</when><who>Iggy</who></comment><comment><id>3</id><package>0ad</package><project>home:Admin</project><when>2021-10-15 13:54:38 UTC</when><who>Iggy</who></comment><comment_count>6</comment_count><user>Iggy</user></xml>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := NewXMLEncoder(nil).EncodeArbitrary(tc.Body)
			assert.NilError(t, err)
			assert.Equal(t, tc.Expected, string(result))

			dec := NewXMLDecoder(nil)
			parsedResult, err := dec.Decode(bytes.NewBuffer([]byte(tc.Expected)), nil)
			assert.NilError(t, err)

			assert.DeepEqual(t, tc.Body, parsedResult)
		})
	}
}
