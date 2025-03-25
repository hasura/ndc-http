package contenttype

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

var errURLEncodedBodyObjectRequired = errors.New("expected object body in content type " + rest.ContentTypeFormURLEncoded)

// URLParameterEncoderOptions hold decode options for the URLParameterEncoder.
type URLParameterEncoderOptions struct {
	StringifyJSON bool
}

// URLParameterEncoder represents a URL parameter encoder.
type URLParameterEncoder struct {
	schema      *rest.NDCHttpSchema
	requestBody *rest.RequestBody
	options     URLParameterEncoderOptions
}

// NewURLParameterEncoder creates a URLParameterEncoder instance.
func NewURLParameterEncoder(schema *rest.NDCHttpSchema, requestBody *rest.RequestBody, options URLParameterEncoderOptions) *URLParameterEncoder {
	return &URLParameterEncoder{
		schema:      schema,
		requestBody: requestBody,
		options:     options,
	}
}

// Encode URL parameters.
func (c *URLParameterEncoder) EncodeFormBody(bodyInfo *rest.ArgumentInfo, bodyData any) ([]byte, error) {
	objectType, bodyObject, err := c.evalRequestBody(bodyInfo.Type, reflect.ValueOf(bodyData))
	if err != nil {
		return nil, err
	}

	if objectType == nil {
		return c.EncodeArbitrary(bodyData)
	}

	q := url.Values{}

	for key, value := range bodyObject {
		objectField, ok := objectType.Fields[key]
		if !ok {
			continue
		}

		queryParams, err := c.EncodeParameterValues(&objectField, reflect.ValueOf(value), []string{"body", key})
		if err != nil {
			return nil, err
		}

		if len(queryParams) == 0 {
			continue
		}

		fieldEncoding := bodyInfo.HTTP.EncodingObject
		if c.requestBody != nil && len(c.requestBody.Encoding) > 0 {
			if enc, ok := c.requestBody.Encoding[key]; ok {
				fieldEncoding = enc
			}
		}

		EvalQueryParameters(&q, key, queryParams, fieldEncoding)
	}

	rawQuery := EncodeQueryValues(q, true)

	return []byte(rawQuery), nil
}

// Encode marshals the arbitrary body to xml bytes.
func (c *URLParameterEncoder) EncodeArbitrary(bodyData any) ([]byte, error) {
	queryParams, err := c.encodeParameterReflectionValues(reflect.ValueOf(bodyData), []string{"body"})
	if err != nil {
		return nil, err
	}

	if len(queryParams) == 0 {
		return nil, nil
	}

	q := url.Values{}
	encObject := rest.EncodingObject{}
	EvalQueryParameters(&q, "", queryParams, encObject)
	rawQuery := EncodeQueryValues(q, true)

	return []byte(rawQuery), nil
}

func (c *URLParameterEncoder) EncodeParameterValues(objectField *rest.ObjectField, reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}

	typeSchema := objectField.HTTP
	reflectValue, notNull := utils.UnwrapPointerFromReflectValue(reflectValue)

	switch ty := objectField.Type.Interface().(type) {
	case *schema.NullableType:
		if !notNull {
			return results, nil
		}

		return c.EncodeParameterValues(&rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: ty.UnderlyingType,
			},
			HTTP: typeSchema,
		}, reflectValue, fieldPaths)
	case *schema.ArrayType:
		if !notNull {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), errArgumentRequired)
		}

		elements, ok := reflectValue.Interface().([]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected array, got <%s> %v", strings.Join(fieldPaths, ""), reflectValue.Kind(), reflectValue.Interface())
		}

		for i, elem := range elements {
			outputs, err := c.EncodeParameterValues(&rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: ty.ElementType,
				},
				HTTP: typeSchema.Items,
			}, reflect.ValueOf(elem), append(fieldPaths, "["+strconv.Itoa(i)+"]"))
			if err != nil {
				return nil, err
			}

			for _, output := range outputs {
				results.Add(append([]Key{NewIndexKey(i)}, output.Keys()...), output.Values())
			}
		}

		return results, nil
	case *schema.NamedType:
		if !notNull {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), errArgumentRequired)
		}

		iScalar, ok := c.schema.ScalarTypes[ty.Name]
		if ok {
			return c.encodeScalarParameterReflectionValues(reflectValue, &iScalar, fieldPaths)
		}

		kind := reflectValue.Kind()

		objectInfo, ok := c.schema.ObjectTypes[ty.Name]
		if !ok {
			return nil, fmt.Errorf("%s: invalid type %s", strings.Join(fieldPaths, ""), ty.Name)
		}

		switch kind {
		case reflect.Map, reflect.Interface:
			anyValue := reflectValue.Interface()
			object, ok := anyValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: failed to evaluate object, got <%s> %v", strings.Join(fieldPaths, ""), kind, anyValue)
			}

			for key, fieldInfo := range objectInfo.Fields {
				fieldVal := object[key]

				output, err := c.EncodeParameterValues(&fieldInfo, reflect.ValueOf(fieldVal), append(fieldPaths, "."+key))
				if err != nil {
					return nil, err
				}

				for _, pair := range output {
					results.Add(append([]Key{NewKey(key)}, pair.Keys()...), pair.Values())
				}
			}
		case reflect.Struct:
			reflectType := reflectValue.Type()
			for fieldIndex := range reflectValue.NumField() {
				fieldVal := reflectValue.Field(fieldIndex)
				fieldType := reflectType.Field(fieldIndex)

				fieldInfo, ok := objectInfo.Fields[fieldType.Name]
				if !ok {
					continue
				}

				output, err := c.EncodeParameterValues(&fieldInfo, fieldVal, append(fieldPaths, "."+fieldType.Name))
				if err != nil {
					return nil, err
				}

				for _, pair := range output {
					results.Add(append([]Key{NewKey(fieldType.Name)}, pair.Keys()...), pair.Values())
				}
			}
		default:
			return nil, fmt.Errorf("%s: failed to evaluate object, got %s", strings.Join(fieldPaths, ""), kind)
		}

		return results, nil
	}

	return nil, fmt.Errorf("%s: invalid type %v", strings.Join(fieldPaths, ""), objectField.Type)
}

func (c *URLParameterEncoder) encodeScalarParameterReflectionValues(reflectValue reflect.Value, scalar *schema.ScalarType, fieldPaths []string) (ParameterItems, error) {
	switch sl := scalar.Representation.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		value, err := utils.DecodeBooleanReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatBool(value)}),
		}, nil
	case *schema.TypeRepresentationString, *schema.TypeRepresentationBytes:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{value})}, nil
	case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64, *schema.TypeRepresentationBigInteger:
		value, err := utils.DecodeIntReflection[int64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatInt(value, 10)}),
		}, nil
	case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64, *schema.TypeRepresentationBigDecimal:
		value, err := utils.DecodeFloatReflection[float64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{fmt.Sprint(value)}),
		}, nil
	case *schema.TypeRepresentationEnum:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		if !slices.Contains(sl.OneOf, value) {
			return nil, fmt.Errorf("%s: the value must be one of %v, got %s", strings.Join(fieldPaths, ""), sl.OneOf, value)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{value})}, nil
	case *schema.TypeRepresentationDate:
		value, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{value.Format(time.DateOnly)}),
		}, nil
	case *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ:
		value, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{value.Format(time.RFC3339)}),
		}, nil
	case *schema.TypeRepresentationUUID:
		rawValue, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		_, err = uuid.Parse(rawValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{rawValue})}, nil
	case *schema.TypeRepresentationJSON:
		if c.options.StringifyJSON {
			// try to evaluate if the value is a json string
			rawValue, err := utils.DecodeStringReflection(reflectValue)
			if err == nil {
				var anyValue any
				if err := json.Unmarshal([]byte(rawValue), &anyValue); err == nil {
					return c.encodeParameterReflectionValues(reflect.ValueOf(anyValue), fieldPaths)
				}
			}
		}

		return c.encodeParameterReflectionValues(reflectValue, fieldPaths)
	default:
		return c.encodeParameterReflectionValues(reflectValue, fieldPaths)
	}
}

func (c *URLParameterEncoder) encodeParameterReflectionValues(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	reflectValue, ok := utils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		return ParameterItems{}, nil
	}

	kind := reflectValue.Kind()
	if c.requestBody != nil && c.requestBody.ContentType == rest.ContentTypeMultipartFormData {
		if result, err := StringifySimpleScalar(reflectValue, kind); err == nil {
			return []ParameterItem{
				NewParameterItem([]Key{}, []string{result}),
			}, nil
		}
	}

	switch kind {
	case reflect.Slice, reflect.Array:
		return c.encodeParameterReflectionSlice(reflectValue, fieldPaths)
	case reflect.Map, reflect.Interface:
		return c.encodeParameterReflectionMap(reflectValue, fieldPaths)
	case reflect.Struct:
		return c.encodeParameterReflectionStruct(reflectValue, fieldPaths)
	default:
		if result, err := StringifySimpleScalar(reflectValue, kind); err == nil {
			return []ParameterItem{
				NewParameterItem([]Key{}, []string{result}),
			}, nil
		}

		return nil, fmt.Errorf("%s: failed to encode parameter, got %s", strings.Join(fieldPaths, ""), kind)
	}
}

func (c *URLParameterEncoder) encodeParameterReflectionSlice(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	valueLen := reflectValue.Len()
	for i := range valueLen {
		elem := reflectValue.Index(i)
		outputs, err := c.encodeParameterReflectionValues(elem, append(fieldPaths, fmt.Sprintf("[%d]", i)))
		if err != nil {
			return nil, err
		}

		for _, output := range outputs {
			results.Add(append([]Key{NewIndexKey(i)}, output.Keys()...), output.Values())
		}
	}

	return results, nil
}

func (c *URLParameterEncoder) encodeParameterReflectionStruct(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	reflectType := reflectValue.Type()
	for fieldIndex := range reflectValue.NumField() {
		fieldVal := reflectValue.Field(fieldIndex)
		fieldType := reflectType.Field(fieldIndex)
		output, err := c.encodeParameterReflectionValues(fieldVal, append(fieldPaths, "."+fieldType.Name))
		if err != nil {
			return nil, err
		}

		for _, pair := range output {
			results.Add(append([]Key{NewKey(fieldType.Name)}, pair.Keys()...), pair.Values())
		}
	}

	return results, nil
}

func (c *URLParameterEncoder) encodeParameterReflectionMap(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	anyValue := reflectValue.Interface()
	object, ok := anyValue.(map[string]any)
	if !ok {
		b, err := json.Marshal(anyValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		values := []string{strings.Trim(string(b), `"`)}

		return []ParameterItem{NewParameterItem([]Key{}, values)}, nil
	}

	for key, fieldValue := range object {
		output, err := c.encodeParameterReflectionValues(reflect.ValueOf(fieldValue), append(fieldPaths, "."+key))
		if err != nil {
			return nil, err
		}

		for _, pair := range output {
			results.Add(append([]Key{NewKey(key)}, pair.Keys()...), pair.Values())
		}
	}

	return results, nil
}

func (c *URLParameterEncoder) evalRequestBody(bodyType schema.Type, bodyData reflect.Value) (*rest.ObjectType, map[string]any, error) {
	reflectValue, notNull := utils.UnwrapPointerFromReflectValue(bodyData)

	switch ty := bodyType.Interface().(type) {
	case *schema.NullableType:
		if !notNull {
			return nil, nil, nil
		}

		return c.evalRequestBody(ty.UnderlyingType, reflectValue)
	case *schema.ArrayType:
		return nil, nil, errURLEncodedBodyObjectRequired
	case *schema.NamedType:
		if !notNull {
			return nil, nil, fmt.Errorf("%s: %w", "body", errArgumentRequired)
		}

		object, ok := reflectValue.Interface().(map[string]any)
		if !ok {
			return nil, nil, errURLEncodedBodyObjectRequired
		}

		objectType, ok := c.schema.ObjectTypes[ty.Name]
		if ok {
			return &objectType, object, nil
		}

		iScalar, ok := c.schema.ScalarTypes[ty.Name]
		if ok {
			if _, err := iScalar.Representation.AsJSON(); err == nil {
				return nil, object, nil
			}
		}
	}

	return nil, nil, errURLEncodedBodyObjectRequired
}

// EvalQueryParameters evaluate the query parameter URL.
func EvalQueryParameters(q *url.Values, name string, queryParams ParameterItems, encObject rest.EncodingObject) {
	explode := encObject.GetExplode(rest.InQuery)

	for _, qp := range queryParams {
		keys := qp.Keys()
		values := qp.Values()

		if len(keys) == 0 {
			if name != "" {
				for _, v := range values {
					q.Add(name, v)
				}
			}

			continue
		}

		paramKey, values := buildURLQueryKeyValues(name, keys, values, encObject)

		if paramKey == "" {
			continue
		}

		for _, v := range values {
			q.Add(paramKey, v)
		}
	}

	// if explode=true, array values will be flatten to multiple key=value items,
	// separated by &
	// /users?id=3&id=4&id=5
	if explode || encObject.Style == rest.EncodingStyleDeepObject {
		return
	}

	for key, values := range *q {
		switch encObject.Style {
		case rest.EncodingStyleSpaceDelimited:
			q.Set(key, strings.Join(values, " "))
		case rest.EncodingStylePipeDelimited:
			q.Set(key, strings.Join(values, "|"))
		case rest.EncodingStyleDeepObject:
		// default style is form
		default:
			q.Set(key, strings.Join(values, ","))
		}
	}
}

func buildURLQueryKeyValues(name string, keys Keys, values []string, encObject rest.EncodingObject) (string, []string) {
	if name != "" {
		keys = append(Keys{NewKey(name)}, keys...)
	}

	if len(keys) == 0 {
		return "", nil
	}

	isDeepObject := encObject.Style == rest.EncodingStyleDeepObject
	isArray := keys[len(keys)-1].Index() != nil

	if isArray {
		return keys.Format(isDeepObject), values
	}

	var resultKey string

	if !isDeepObject {
		resultKey = keys[0].String()

		keys = keys[1:]
	}

	builtKey := keys.Format(isDeepObject)
	// if explode=false, the root key will be excluded.
	// Object id = {“role”: “admin”, “firstName”: “Alex”}
	// => /users?role=admin&firstName=Alex
	if isDeepObject || encObject.GetExplode(rest.InQuery) {
		return builtKey, values
	}

	// if explode=false, child keys will be in query values.
	// Object id = {“role”: “admin”, “firstName”: “Alex”}
	// => /users?id=role,admin,firstName,Alex
	if builtKey != "" {
		values = append([]string{builtKey}, values...)
	}

	return resultKey, values
}

// EncodeQueryValues encode query values to string.
func EncodeQueryValues(qValues url.Values, allowReserved bool) string {
	if !allowReserved {
		return qValues.Encode()
	}

	var builder strings.Builder
	index := 0
	for key, values := range qValues {
		for i, value := range values {
			if index > 0 || i > 0 {
				builder.WriteRune('&')
			}
			builder.WriteString(key)
			builder.WriteRune('=')
			builder.WriteString(value)
		}
		index++
	}

	return builder.String()
}

// SetHeaderParameters encode and set parameters to request headers.
func SetHeaderParameters(header *http.Header, param *rest.RequestParameter, queryParams ParameterItems) {
	defaultParam := queryParams.FindDefault()
	// the param is an array
	if defaultParam != nil {
		header.Set(param.Name, strings.Join(defaultParam.Values(), ","))

		return
	}

	explode := param.GetExplode(rest.InHeader)
	headerValues := transformParameterItemStrings(queryParams, explode)
	header.Set(param.Name, strings.Join(headerValues, ","))
}

// EncodePathParameters encode parameters to the request path.
func EncodePathParameters(rawPath string, name string, queryParams ParameterItems, enc rest.EncodingObject) string {
	value := encodePathParameterValue(name, queryParams, enc)

	return strings.ReplaceAll(rawPath, "{"+name+"}", value)
}

func encodePathParameterValue(name string, queryParams ParameterItems, enc rest.EncodingObject) string {
	style := enc.GetStyle(rest.InPath)
	explode := enc.GetExplode(rest.InPath)

	defaultParam := queryParams.FindDefault()
	// the param is an array or a primitive value
	if defaultParam != nil {
		values := defaultParam.Values()

		if len(values) == 0 || (len(values) == 1 && values[0] == "") {
			switch style {
			case rest.EncodingStyleMatrix:
				return ";" + name
			case rest.EncodingStyleLabel:
				return "."
			default:
				return ""
			}
		}

		switch style {
		case rest.EncodingStyleMatrix:
			if !explode {
				// ;color=blue,black,brown
				return ";" + name + "=" + strings.Join(values, ",")
			}

			// ;color=blue;color=black;color=brown
			var sb strings.Builder

			for _, value := range values {
				sb.WriteRune(';')
				sb.WriteString(name)
				sb.WriteRune('=')
				sb.WriteString(value)
			}

			return sb.String()
		case rest.EncodingStyleLabel:
			// .blue,black,brown
			if explode {
				return "." + strings.Join(values, ".")
			}

			return "." + strings.Join(values, ",")
		default:
			// blue,black,brown
			return strings.Join(values, ",")
		}
	}

	keyValues := transformParameterItemStrings(queryParams, explode)

	switch style {
	case rest.EncodingStyleMatrix:
		if explode {
			// ;R=100;G=200;B=150
			return ";" + strings.Join(keyValues, ";")
		}

		// ;color=R,100,G,200,B,150
		return ";" + name + "=" + strings.Join(keyValues, ",")
	case rest.EncodingStyleLabel:
		// .blue,black,brown
		if explode {
			return "." + strings.Join(keyValues, ".")
		}

		return "." + strings.Join(keyValues, ",")
	default:
		// blue,black,brown
		return strings.Join(keyValues, ",")
	}
}

func transformParameterItemStrings(queryParams ParameterItems, explode bool) []string {
	var headerValues []string

	for _, pair := range queryParams {
		key := pair.Keys().Format(false)
		for _, value := range pair.Values() {
			if explode {
				// R=100,G=200,B=150
				headerValues = append(headerValues, key+"="+value)

				continue
			}

			// R,100,G,200,B,150
			headerValues = append(headerValues, key, value)
		}
	}

	return headerValues
}
