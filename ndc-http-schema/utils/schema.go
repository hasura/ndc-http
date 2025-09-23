package utils

import (
	"fmt"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/schema"
)

// UnwrapNullableType unwraps the underlying type of the nullable type.
func UnwrapNullableType(input schema.Type) (schema.TypeEncoder, bool, error) {
	return UnwrapNullableTypeEncoder(input.Interface())
}

// UnwrapNullableTypeEncoder unwraps the underlying type of the nullable type.
func UnwrapNullableTypeEncoder(input schema.TypeEncoder) (schema.TypeEncoder, bool, error) {
	switch ty := input.(type) {
	case *schema.NullableType:
		childType, _, err := UnwrapNullableType(ty.UnderlyingType)
		if err != nil {
			return nil, false, err
		}

		return childType, true, nil
	case *schema.NamedType, *schema.ArrayType:
		return ty, false, nil
	default:
		return nil, false, fmt.Errorf("invalid type %v", input)
	}
}

// WrapNullableTypeEncoder wraps the schema type with nullable.
func WrapNullableTypeEncoder(input schema.TypeEncoder) schema.TypeEncoder {
	if !IsNullableTypeEncoder(input) {
		return schema.NewNullableType(input)
	}

	return input
}

// IsNullableTypeEncoder checks if the input type is nullable.
func IsNullableTypeEncoder(input schema.TypeEncoder) bool {
	_, ok := input.(*schema.NullableType)

	return ok
}

// BuildUniqueSchemaTypeName builds the unique type name from schema.
func BuildUniqueSchemaTypeName(sm *rest.NDCHttpSchema, name string) string {
	return buildUniqueSchemaTypeName(sm, name, 0)
}

func buildUniqueSchemaTypeName(sm *rest.NDCHttpSchema, name string, times int) string {
	newName := name

	if times > 0 {
		newName += strconv.Itoa(times)
	}

	lowerName := strings.ToLower(newName)

	for key := range sm.ObjectTypes {
		if lowerName == strings.ToLower(key) {
			return buildUniqueSchemaTypeName(sm, name, times+1)
		}
	}

	for key := range sm.ScalarTypes {
		if lowerName == strings.ToLower(key) {
			return buildUniqueSchemaTypeName(sm, name, times+1)
		}
	}

	return newName
}
