package contenttype

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// JSONEncoder implements a dynamic JSON encode from the HTTP schema.
type JSONEncoder struct {
	schema *rest.NDCHttpSchema
	buffer *bytes.Buffer
}

// NewJSONEncoder creates a new JSON encoder.
func NewJSONEncoder(httpSchema *rest.NDCHttpSchema) *JSONEncoder {
	return &JSONEncoder{
		schema: httpSchema,
	}
}

// Encode unmarshals json and evaluate the schema type.
func (c *JSONEncoder) Encode(input any, resultType schema.Type) ([]byte, error) {
	c.buffer = &bytes.Buffer{}
	if err := c.evalSchemaType(reflect.ValueOf(input), resultType, []string{}); err != nil {
		return nil, err
	}

	return c.buffer.Bytes(), nil
}

func (c *JSONEncoder) evalSchemaType(
	reflectValue reflect.Value,
	schemaType schema.Type,
	fieldPaths []string,
) error {
	rawType, err := schemaType.InterfaceT()
	if err != nil {
		return err
	}

	reflectValue, notNull := utils.UnwrapPointerFromReflectValue(reflectValue)

	switch t := rawType.(type) {
	case *schema.NullableType:
		if !notNull {
			c.buffer.WriteString("null")

			return nil
		}

		return c.evalSchemaType(reflectValue, t.UnderlyingType, fieldPaths)
	case *schema.ArrayType:
		return c.evalArrayType(reflectValue, t, fieldPaths)
	case *schema.NamedType:
		return c.evalNamedType(reflectValue, t, fieldPaths)
	default:
		return nil
	}
}

func (c *JSONEncoder) evalArrayType(
	reflectValue reflect.Value,
	arrayType *schema.ArrayType,
	fieldPaths []string,
) error {
	kind := reflectValue.Kind()
	if kind != reflect.Slice && kind != reflect.Array {
		return fmt.Errorf(
			"%s: expected array, got %v",
			strings.Join(fieldPaths, "."),
			reflectValue.Kind(),
		)
	}

	c.buffer.WriteRune('[')
	valueLen := reflectValue.Len()

	for i := range valueLen {
		reflectElem := reflectValue.Index(i)

		err := c.evalSchemaType(
			reflectElem,
			arrayType.ElementType,
			append(fieldPaths, strconv.Itoa(i)),
		)
		if err != nil {
			return err
		}

		if i < valueLen-1 {
			c.buffer.WriteRune(',')
		}
	}

	c.buffer.WriteRune(']')

	return nil
}

func (c *JSONEncoder) evalNamedType(
	reflectValue reflect.Value,
	schemaType *schema.NamedType,
	fieldPaths []string,
) error {
	scalarType, ok := c.schema.ScalarTypes[schemaType.Name]
	if ok {
		err := c.evalScalarType(reflectValue, scalarType)
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		return nil
	}

	objectType, ok := c.schema.ObjectTypes[schemaType.Name]
	if !ok {
		return nil
	}

	objectValue, ok := reflectValue.Interface().(map[string]any)
	if !ok {
		return fmt.Errorf(
			"%s: expected object, got %v",
			strings.Join(fieldPaths, "."),
			reflectValue.Kind(),
		)
	}

	var started bool
	c.buffer.WriteRune('{')

	for key, field := range objectType.Fields {
		fieldValue, ok := objectValue[key]
		if !ok {
			continue
		}

		if !started {
			started = true
		} else {
			c.buffer.WriteRune(',')
		}

		c.writeStringValue(key)
		c.buffer.WriteRune(':')

		err := c.evalSchemaType(reflect.ValueOf(fieldValue), field.Type, append(fieldPaths, key))
		if err != nil {
			return err
		}
	}

	c.buffer.WriteRune('}')

	return nil
}

func (c *JSONEncoder) evalScalarType(
	reflectValue reflect.Value,
	scalarType schema.ScalarType,
) error {
	switch rep := scalarType.Representation.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		boolValue, err := utils.DecodeBooleanReflection(reflectValue)
		if err != nil {
			return err
		}

		c.buffer.WriteString(strconv.FormatBool(boolValue))
	case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64:
		value, err := utils.DecodeFloatReflection[float64](reflectValue)
		if err != nil {
			return err
		}

		c.buffer.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	case *schema.TypeRepresentationBigDecimal:
		value, err := utils.DecodeFloatReflection[float64](reflectValue)
		if err != nil {
			return err
		}

		c.writeStringValue(strconv.FormatFloat(value, 'f', -1, 64))
	case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64:
		value, err := utils.DecodeIntReflection[int64](reflectValue)
		if err != nil {
			return err
		}

		c.buffer.WriteString(strconv.FormatInt(value, 10))
	case *schema.TypeRepresentationBigInteger:
		value, err := utils.DecodeIntReflection[int64](reflectValue)
		if err != nil {
			return err
		}

		c.writeStringValue(strconv.FormatInt(value, 10))
	case *schema.TypeRepresentationString, *schema.TypeRepresentationBytes:
		str, err := StringifySimpleScalar(reflectValue, reflectValue.Kind())
		if err != nil {
			return err
		}

		c.buffer.WriteString(strconv.Quote(str))
	case *schema.TypeRepresentationUUID:
		str, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return err
		}

		if err := uuid.Validate(str); err != nil {
			return err
		}

		c.writeStringValue(str)
	case *schema.TypeRepresentationEnum:
		str, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return err
		}

		if !slices.Contains(rep.OneOf, str) {
			return fmt.Errorf("expected one of %v, got: %s", rep.OneOf, str)
		}

		c.writeStringValue(str)
	case *schema.TypeRepresentationDate:
		d, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return err
		}

		c.writeStringValue(d.Format(time.DateOnly))
	case *schema.TypeRepresentationTimestamp:
		d, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return err
		}

		c.writeStringValue(d.Format("2006-01-02T15:04:05Z"))
	case *schema.TypeRepresentationTimestampTZ:
		d, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return err
		}

		c.writeStringValue(d.Format(time.RFC3339))
	default:
		str, err := utils.DecodeStringReflection(reflectValue)
		if err == nil {
			jsonBytes := []byte(str)
			if json.Valid(jsonBytes) {
				c.buffer.Write(jsonBytes)

				return nil
			}
		}

		jsonBytes, err := json.Marshal(reflectValue.Interface())
		if err != nil {
			return err
		}

		c.buffer.Write(jsonBytes)
	}

	return nil
}

func (c *JSONEncoder) writeStringValue(str string) {
	c.buffer.WriteRune('"')
	c.buffer.WriteString(str)
	c.buffer.WriteRune('"')
}
