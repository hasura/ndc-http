package contenttype

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// JSONDecodeOptions hold decode options for the JSONDecoder.
type JSONDecodeOptions struct {
	StringifyJSON bool
}

// JSONDecoder implements a dynamic JSON decoder from the HTTP schema.
type JSONDecoder struct {
	schema  *rest.NDCHttpSchema
	options JSONDecodeOptions
}

// NewJSONDecoder creates a new JSON encoder.
func NewJSONDecoder(httpSchema *rest.NDCHttpSchema, options JSONDecodeOptions) *JSONDecoder {
	return &JSONDecoder{
		schema:  httpSchema,
		options: options,
	}
}

// Decode unmarshals json and evaluate the schema type.
func (c *JSONDecoder) Decode(r io.Reader, resultType schema.Type) (any, error) {
	underlyingType, _, err := restUtils.UnwrapNullableType(resultType)
	if err != nil {
		return nil, err
	}

	switch t := underlyingType.(type) {
	case *schema.ArrayType:
		var rawResult []any
		err := json.NewDecoder(r).Decode(&rawResult)
		if err != nil {
			return nil, err
		}

		if utils.IsNil(rawResult) {
			return nil, nil
		}

		return c.evalArrayType(rawResult, t, []string{})
	case *schema.NamedType:
		var result any
		err := json.NewDecoder(r).Decode(&result)
		if err != nil {
			return nil, err
		}

		if utils.IsNil(result) {
			return nil, nil
		}

		return c.evalNamedType(result, t, []string{})
	default:
		var result any
		err := json.NewDecoder(r).Decode(&result)

		return result, err
	}
}

func (c *JSONDecoder) evalSchemaType(
	value any,
	schemaType schema.Type,
	fieldPaths []string,
) (any, error) {
	if utils.IsNil(value) {
		return nil, nil
	}

	switch t := schemaType.Interface().(type) {
	case *schema.NullableType:
		return c.evalSchemaType(value, t.UnderlyingType, fieldPaths)
	case *schema.ArrayType:
		return c.evalArrayType(value, t, fieldPaths)
	case *schema.NamedType:
		return c.evalNamedType(value, t, fieldPaths)
	default:
		return value, nil
	}
}

func (c *JSONDecoder) evalArrayType(
	value any,
	arrayType *schema.ArrayType,
	fieldPaths []string,
) (any, error) {
	arrayValue, ok := value.([]any)
	if !ok {
		return value, nil
	}

	results := make([]any, len(arrayValue))
	for i, item := range arrayValue {
		result, err := c.evalSchemaType(
			item,
			arrayType.ElementType,
			append(fieldPaths, strconv.Itoa(i)),
		)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}

	return results, nil
}

func (c *JSONDecoder) evalNamedType(
	value any,
	schemaType *schema.NamedType,
	fieldPaths []string,
) (any, error) {
	scalarType, ok := c.schema.ScalarTypes[schemaType.Name]
	if ok {
		result, err := c.evalScalarType(value, scalarType)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		return result, nil
	}

	objectType, ok := c.schema.ObjectTypes[schemaType.Name]
	if !ok {
		return value, nil
	}

	objectValue, ok := value.(map[string]any)
	if !ok {
		return value, nil
	}

	results := make(map[string]any)
	for key, field := range objectType.Fields {
		fieldValue, ok := objectValue[key]
		if !ok {
			continue
		}

		if fieldValue == nil {
			results[key] = nil

			continue
		}

		result, err := c.evalSchemaType(fieldValue, field.Type, append(fieldPaths, key))
		if err != nil {
			return nil, err
		}

		results[key] = result
	}

	return results, nil
}

func (c *JSONDecoder) evalScalarType(value any, scalarType schema.ScalarType) (any, error) {
	switch scalarType.Representation.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		return utils.DecodeBoolean(value)
	case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64:
		return utils.DecodeFloat[float64](value)
	case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64:
		return utils.DecodeInt[int64](value)
	case *schema.TypeRepresentationString:
		if s, ok := value.(string); ok {
			return s, nil
		}

		reflectType := reflect.ValueOf(value)

		return StringifySimpleScalar(reflectType, reflectType.Kind())
	case *schema.TypeRepresentationJSON:
		if c.options.StringifyJSON {
			jsonBytes, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}

			return string(jsonBytes), nil
		}

		return value, nil
	default:
		return value, nil
	}
}
