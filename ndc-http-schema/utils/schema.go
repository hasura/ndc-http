package utils

import (
	"fmt"

	"github.com/hasura/ndc-sdk-go/schema"
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

// WrapNullableType wraps the schema type with nullable.
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
