package utils

import (
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"gotest.tools/v3/assert"
)

func TestUnwrapNullableType(t *testing.T) {
	testCases := []struct {
		Name           string
		Input          schema.TypeEncoder
		ExpectedType   schema.TypeEncoder
		ExpectedNullok bool
		ExpectError    bool
	}{
		{
			Name:           "nullable_named_type",
			Input:          schema.NewNullableNamedType("String"),
			ExpectedType:   schema.NewNamedType("String"),
			ExpectedNullok: true,
			ExpectError:    false,
		},
		{
			Name:           "non_nullable_named_type",
			Input:          schema.NewNamedType("String"),
			ExpectedType:   schema.NewNamedType("String"),
			ExpectedNullok: false,
			ExpectError:    false,
		},
		{
			Name:           "nullable_array_type",
			Input:          schema.NewNullableType(schema.NewArrayType(schema.NewNamedType("String"))),
			ExpectedType:   schema.NewArrayType(schema.NewNamedType("String")),
			ExpectedNullok: true,
			ExpectError:    false,
		},
		{
			Name:           "non_nullable_array_type",
			Input:          schema.NewArrayType(schema.NewNamedType("String")),
			ExpectedType:   schema.NewArrayType(schema.NewNamedType("String")),
			ExpectedNullok: false,
			ExpectError:    false,
		},
		{
			Name: "nested_nullable",
			Input: schema.NewNullableType(
				schema.NewNullableType(schema.NewNamedType("String")),
			),
			ExpectedType:   schema.NewNamedType("String"),
			ExpectedNullok: true,
			ExpectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			resultType, nullable, err := UnwrapNullableType(tc.Input.Encode())

			if tc.ExpectError {
				assert.Error(t, err, "expected an error")
			} else {
				assert.NilError(t, err)
				assert.Equal(t, tc.ExpectedNullok, nullable)
				assert.DeepEqual(t, tc.ExpectedType, resultType)
			}
		})
	}
}

func TestUnwrapNullableTypeEncoder(t *testing.T) {
	testCases := []struct {
		Name           string
		Input          schema.TypeEncoder
		ExpectedType   schema.TypeEncoder
		ExpectedNullok bool
		ExpectError    bool
	}{
		{
			Name:           "nullable_type",
			Input:          schema.NewNullableNamedType("String"),
			ExpectedType:   schema.NewNamedType("String"),
			ExpectedNullok: true,
			ExpectError:    false,
		},
		{
			Name:           "named_type",
			Input:          schema.NewNamedType("String"),
			ExpectedType:   schema.NewNamedType("String"),
			ExpectedNullok: false,
			ExpectError:    false,
		},
		{
			Name:           "array_type",
			Input:          schema.NewArrayType(schema.NewNamedType("String")),
			ExpectedType:   schema.NewArrayType(schema.NewNamedType("String")),
			ExpectedNullok: false,
			ExpectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			resultType, nullable, err := UnwrapNullableTypeEncoder(tc.Input)

			if tc.ExpectError {
				assert.Error(t, err, "expected an error")
			} else {
				assert.NilError(t, err)
				assert.Equal(t, tc.ExpectedNullok, nullable)
				assert.DeepEqual(t, tc.ExpectedType, resultType)
			}
		})
	}
}

func TestWrapNullableTypeEncoder(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    schema.TypeEncoder
		Expected schema.TypeEncoder
	}{
		{
			Name:     "named_type",
			Input:    schema.NewNamedType("String"),
			Expected: schema.NewNullableNamedType("String"),
		},
		{
			Name:     "already_nullable",
			Input:    schema.NewNullableNamedType("String"),
			Expected: schema.NewNullableNamedType("String"),
		},
		{
			Name:     "array_type",
			Input:    schema.NewArrayType(schema.NewNamedType("String")),
			Expected: schema.NewNullableType(schema.NewArrayType(schema.NewNamedType("String"))),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := WrapNullableTypeEncoder(tc.Input)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}

func TestIsNullableTypeEncoder(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    schema.TypeEncoder
		Expected bool
	}{
		{
			Name:     "nullable_type",
			Input:    schema.NewNullableNamedType("String"),
			Expected: true,
		},
		{
			Name:     "named_type",
			Input:    schema.NewNamedType("String"),
			Expected: false,
		},
		{
			Name:     "array_type",
			Input:    schema.NewArrayType(schema.NewNamedType("String")),
			Expected: false,
		},
		{
			Name:     "nullable_array_type",
			Input:    schema.NewNullableType(schema.NewArrayType(schema.NewNamedType("String"))),
			Expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsNullableTypeEncoder(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestBuildUniqueSchemaTypeName(t *testing.T) {
	testCases := []struct {
		Name     string
		Schema   *rest.NDCHttpSchema
		Input    string
		Expected string
	}{
		{
			Name: "no_conflict",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{
					"User": {},
				},
				ScalarTypes: map[string]schema.ScalarType{
					"CustomInt": {},
				},
			},
			Input:    "Product",
			Expected: "Product",
		},
		{
			Name: "conflict_with_object_type",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{
					"User": {},
				},
				ScalarTypes: map[string]schema.ScalarType{},
			},
			Input:    "User",
			Expected: "User1",
		},
		{
			Name: "conflict_with_scalar_type",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{},
				ScalarTypes: map[string]schema.ScalarType{
					"CustomInt": {},
				},
			},
			Input:    "CustomInt",
			Expected: "CustomInt1",
		},
		{
			Name: "conflict_case_insensitive",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{
					"user": {},
				},
				ScalarTypes: map[string]schema.ScalarType{},
			},
			Input:    "User",
			Expected: "User1",
		},
		{
			Name: "multiple_conflicts",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{
					"User":  {},
					"User1": {},
					"User2": {},
				},
				ScalarTypes: map[string]schema.ScalarType{},
			},
			Input:    "User",
			Expected: "User3",
		},
		{
			Name: "empty_schema",
			Schema: &rest.NDCHttpSchema{
				ObjectTypes: map[string]rest.ObjectType{},
				ScalarTypes: map[string]schema.ScalarType{},
			},
			Input:    "NewType",
			Expected: "NewType",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := BuildUniqueSchemaTypeName(tc.Schema, tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}
