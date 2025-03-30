package internal

import (
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestResponseTransformer(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    any
		Expected any
		Setting  rest.ResponseTransformSetting
	}{
		{
			Name: "object_field",
			Input: map[string]any{
				"data": map[string]any{
					"foo": "bar",
				},
			},
			Expected: map[string]any{
				"foo": "bar",
			},
			Setting: rest.ResponseTransformSetting{
				Body: "$.data",
			},
		},
		{
			Name: "nested_array",
			Input: any(map[string]any{
				"data": []any{
					map[string]any{
						"foo": "bar",
					},
				},
			}),
			Expected: []any{"bar"},
			Setting: rest.ResponseTransformSetting{
				Body: "$.data[*].foo",
			},
		},
		{
			Name: "nested_object_array",
			Input: any(map[string]any{
				"data": []any{
					map[string]any{
						"foo": "bar",
						"user": map[string]any{
							"id": float64(1),
						},
					},
				},
			}),
			Expected: float64(1),
			Setting: rest.ResponseTransformSetting{
				Body: "$.data[0].user.id",
			},
		},
		{
			Name: "literal_selector",
			Input: any(map[string]any{
				"data": []any{
					map[string]any{
						"foo": "bar",
						"user": map[string]any{
							"id": float64(1),
						},
					},
				},
			}),
			Expected: []any{
				map[string]any{
					"id":      10,
					"tags":    []any{"bar"},
					"user_id": float64(1),
					"active":  true,
				},
			},
			Setting: rest.ResponseTransformSetting{
				Body: []any{
					map[string]any{
						"id":      10,
						"tags":    "$.data[*].foo",
						"user_id": "$.data[0].user.id",
						"active":  true,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := NewResponseTransformer(tc.Setting, true).Transform(tc.Input)
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}
