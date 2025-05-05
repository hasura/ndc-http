package internal

import (
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// ResponseTransformer is a processor to evaluate schemas for the response transformation setting.
type ResponseTransformer struct {
	schema  *rest.NDCHttpSchema
	setting rest.ResponseTransformSetting
	logger  *slog.Logger
}

// NewResponseTransformer create a new ResponseTransformer instance.
func NewResponseTransformer(
	ndcSchema *rest.NDCHttpSchema,
	setting rest.ResponseTransformSetting,
	logger *slog.Logger,
) *ResponseTransformer {
	if logger == nil {
		logger = slog.Default()
	}

	return &ResponseTransformer{
		schema:  ndcSchema,
		setting: setting,
		logger:  logger,
	}
}

// Transform evaluates new result types of operations after being transformed.
func (rt *ResponseTransformer) Transform() (*rest.NDCHttpSchema, []string, error) {
	var operationNames []string
	var err error

	targets, err := rt.validateTargets(rt.setting.Targets)
	if err != nil {
		return nil, nil, err
	}

	operationNames, err = rt.transformOperations(
		targets,
		reflect.ValueOf(rt.setting.Body),
		len(rt.setting.Targets) > 0,
	)

	return rt.schema, operationNames, err
}

func (rt *ResponseTransformer) validateTargets(targetInputs []string) ([]string, error) {
	var operationNames []string

	targetRegexes := make([]*regexp.Regexp, len(targetInputs))

	for i, target := range targetInputs {
		regex, err := regexp.Compile(target)
		if err != nil {
			return nil, fmt.Errorf("invalid target pattern at %d: %w", i, err)
		}

		targetRegexes[i] = regex
	}

	for name := range rt.schema.Functions {
		isValid := true

		for _, regex := range targetRegexes {
			isValid = regex.MatchString(name)

			if isValid {
				break
			}
		}

		if isValid {
			operationNames = append(operationNames, name)
		}
	}

	for name := range rt.schema.Procedures {
		isValid := true

		for _, regex := range targetRegexes {
			isValid = regex.MatchString(name)

			if isValid {
				break
			}
		}

		if isValid {
			operationNames = append(operationNames, name)
		}
	}

	return operationNames, nil
}

func (rt *ResponseTransformer) transformOperations(
	operationNames []string,
	body reflect.Value,
	strict bool,
) ([]string, error) {
	appliedNames := []string{}

	for _, name := range operationNames {
		if fn, ok := rt.schema.Functions[name]; ok {
			newOp, err := rt.transformOperation(name, fn, body)
			if err != nil {
				if strict {
					return nil, err
				}

				rt.logger.Debug(
					fmt.Sprintf("failed to transform operation %s: %s", name, err.Error()),
				)

				continue
			}

			rt.schema.Functions[name] = *newOp
			appliedNames = append(appliedNames, name)

			continue
		}

		if proc, ok := rt.schema.Procedures[name]; ok {
			newOp, err := rt.transformOperation(name, proc, body)
			if err != nil {
				if strict {
					return nil, err
				}

				rt.logger.Debug(
					fmt.Sprintf("failed to transform operation %s: %s", name, err.Error()),
				)

				continue
			}

			rt.schema.Procedures[name] = *newOp
			appliedNames = append(appliedNames, name)

			continue
		}

		return nil, fmt.Errorf("failed to transform the operation `%s`: not found", name)
	}

	return appliedNames, nil
}

func (rt *ResponseTransformer) transformOperation(
	opName string,
	op rest.OperationInfo,
	body reflect.Value,
) (*rest.OperationInfo, error) {
	op.BackupResultType()

	newResultType, err := rt.evalResultType(op.ResultType, body, []string{opName})
	if err != nil {
		return nil, err
	}

	op.ResultType = newResultType.Encode()

	return &op, nil
}

func (rt *ResponseTransformer) evalResultType(
	schemaType schema.Type,
	field reflect.Value,
	fieldPaths []string,
) (schema.TypeEncoder, error) {
	field, notNull := utils.UnwrapPointerFromReflectValue(field)
	fieldKind := field.Kind()

	var resultType schema.TypeEncoder

	switch fieldKind {
	case reflect.Bool:
		resultType = schema.NewNamedType(string(rest.ScalarBoolean))
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32:
		resultType = schema.NewNamedType(string(rest.ScalarInt32))
	case reflect.Int64, reflect.Uint64:
		resultType = schema.NewNamedType(string(rest.ScalarInt64))
	case reflect.Float32:
		resultType = schema.NewNamedType(string(rest.ScalarFloat32))
	case reflect.Float64:
		resultType = schema.NewNamedType(string(rest.ScalarFloat64))
	case reflect.String:
		var err error
		resultType, err = rt.evalStringValue(schemaType, field.String(), fieldPaths)
		if err != nil {
			return nil, err
		}
	case reflect.Slice, reflect.Array:
		if !notNull {
			return schema.NewNullableNamedType(string(rest.ScalarJSON)), nil
		}

		if field.Len() != 1 {
			return schema.NewArrayType(schema.NewNullableNamedType(string(rest.ScalarJSON))), nil
		}

		elemType, err := rt.evalResultType(schemaType, field.Index(0), append(fieldPaths, "[]"))
		if err != nil {
			return nil, err
		}

		resultType = schema.NewArrayType(elemType)
	case reflect.Map:
		if !notNull {
			return schema.NewNullableNamedType(string(rest.ScalarJSON)), nil
		}

		keys := field.MapKeys()

		if len(keys) == 0 {
			return schema.NewNamedType(string(rest.ScalarJSON)), nil
		}

		newObjectType := rest.ObjectType{
			Fields: make(map[string]rest.ObjectField),
		}

		for _, rKey := range keys {
			keyStr, err := utils.DecodeStringReflection(rKey)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}

			rValue := field.MapIndex(rKey)
			newFieldType, err := rt.evalResultType(schemaType, rValue, append(fieldPaths, keyStr))
			if err != nil {
				return nil, err
			}

			newObjectType.Fields[keyStr] = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: newFieldType.Encode(),
				},
			}
		}

		newObjectTypeName := restUtils.StringSliceToPascalCase(
			append(fieldPaths, "TransformedResult"),
		)
		rt.schema.ObjectTypes[newObjectTypeName] = newObjectType

		resultType = schema.NewNamedType(newObjectTypeName)
	case reflect.Interface:
		var err error
		value := field.Interface()

		if str, ok := value.(string); ok {
			resultType, err = rt.evalStringValue(schemaType, str, fieldPaths)
			if err != nil {
				return nil, err
			}
		} else if mapValue, ok := value.(map[string]any); ok {
			if len(mapValue) == 0 {
				return schema.NewNamedType(string(rest.ScalarJSON)), nil
			}

			newObjectType := rest.ObjectType{
				Fields: make(map[string]rest.ObjectField),
			}

			for key, v := range mapValue {
				newFieldType, err := rt.evalResultType(schemaType, reflect.ValueOf(v), append(fieldPaths, key))
				if err != nil {
					return nil, err
				}

				newObjectType.Fields[key] = rest.ObjectField{
					ObjectField: schema.ObjectField{
						Type: newFieldType.Encode(),
					},
				}
			}

			newObjectTypeName := restUtils.StringSliceToPascalCase(append(fieldPaths, "TransformedResult"))
			rt.schema.ObjectTypes[newObjectTypeName] = newObjectType

			resultType = schema.NewNamedType(newObjectTypeName)
		} else {
			return nil, fmt.Errorf("unsupported reflection kind %v: %v", fieldKind, field.Interface())
		}

	default:
		return nil, fmt.Errorf(
			"%s: unsupported reflection kind %v: %v",
			strings.Join(fieldPaths, "."),
			fieldKind,
			field.Interface(),
		)
	}

	if !notNull {
		return restUtils.WrapNullableTypeEncoder(resultType), nil
	}

	return resultType, nil
}

func (rt *ResponseTransformer) evalStringValue(
	schemaType schema.Type,
	value string,
	fieldPaths []string,
) (schema.TypeEncoder, error) {
	selector, err := jsonpath.Parse(value)
	if err != nil {
		return schema.NewNamedType(string(rest.ScalarString)), nil //nolint:nilerr
	}

	return rt.evalJSONPath(schemaType, selector.Query().Segments(), fieldPaths)
}

func (rt *ResponseTransformer) evalJSONPath(
	resultType schema.Type,
	segments []*spec.Segment,
	fieldPaths []string,
) (schema.TypeEncoder, error) {
	rawType, err := resultType.InterfaceT()
	if err != nil {
		return nil, err
	}

	if len(segments) == 0 || len(segments[0].Selectors()) == 0 {
		return rawType, nil
	}

	selector := segments[0].Selectors()[0]

	switch t := rawType.(type) {
	case *schema.NullableType:
		underlyingType, err := rt.evalJSONPath(t.UnderlyingType, segments, fieldPaths)
		if err != nil {
			return nil, err
		}

		return restUtils.WrapNullableTypeEncoder(underlyingType), nil
	case *schema.ArrayType:
		switch selector.(type) {
		case spec.WildcardSelector, spec.SliceSelector:
			newType, err := rt.evalJSONPath(t.ElementType, segments[1:], append(fieldPaths, "[]"))
			if err != nil {
				return nil, err
			}

			return schema.NewArrayType(newType), nil
		case spec.Index:
			newType, err := rt.evalJSONPath(t.ElementType, segments[1:], append(fieldPaths, "[]"))
			if err != nil {
				return nil, err
			}

			return schema.NewNullableType(newType), nil
		default:
			return nil, fmt.Errorf("invalid json path at %s. Expected array, got: %v", strings.Join(fieldPaths, "."), selector)
		}
	case *schema.NamedType:
		if scalarType, ok := rt.schema.ScalarTypes[t.Name]; ok {
			if len(segments[0].Selectors()) > 0 {
				if _, err := scalarType.Representation.AsJSON(); err != nil {
					return nil, fmt.Errorf("invalid json path at %s. Cannot select nested fields from primitive scalars", strings.Join(fieldPaths, "."))
				}
			}

			return rawType, nil
		}

		selectorName, isNameSelector := selector.(spec.Name)
		if !isNameSelector {
			return nil, fmt.Errorf("invalid json path at %s. Expected object field name, got: %v", strings.Join(fieldPaths, "."), selector)
		}

		objectType, ok := rt.schema.ObjectTypes[t.Name]
		if !ok {
			return nil, fmt.Errorf("%s: type name %s does not exist", strings.Join(fieldPaths, "."), t.Name)
		}

		objectField, ok := objectType.Fields[string(selectorName)]
		if !ok {
			return nil, fmt.Errorf("invalid json path at %s. Object field name `%s` does not exist", strings.Join(fieldPaths, "."), string(selectorName))
		}

		if len(segments) == 1 {
			return objectField.Type.Interface(), nil
		}

		return rt.evalJSONPath(objectField.Type, segments[1:], append(fieldPaths, string(selectorName)))
	default:
		return nil, fmt.Errorf("%s: invalid type %v", strings.Join(fieldPaths, "."), rawType)
	}
}
