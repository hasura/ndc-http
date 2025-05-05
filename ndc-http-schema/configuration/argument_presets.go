package configuration

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// ValidateArgumentPreset validates the argument preset.
func ValidateArgumentPreset(
	httpSchema *rest.NDCHttpSchema,
	preset rest.ArgumentPresetConfig,
	isGlobal bool,
) (*jsonpath.Path, map[string]schema.TypeRepresentation, error) {
	jsonPath, targetExpressions, err := preset.Validate()
	if err != nil {
		return nil, nil, err
	}

	exprLen := len(targetExpressions)
	targets := make(map[string]schema.TypeRepresentation)
	for key, op := range httpSchema.Functions {
		if exprLen > 0 && !slices.ContainsFunc(targetExpressions, func(expr regexp.Regexp) bool {
			return expr.MatchString(key)
		}) {
			continue
		}

		typeRep, err := evalTypeRepresentationFromJSONPath(httpSchema, jsonPath, &op, isGlobal)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", key, err)
		}

		if typeRep == nil {
			continue
		}

		httpSchema.Functions[key] = op
		targets[BuildArgumentPresetJSONPathKey(key, jsonPath)] = typeRep
	}

	for key, op := range httpSchema.Procedures {
		if exprLen > 0 && !slices.ContainsFunc(targetExpressions, func(expr regexp.Regexp) bool {
			return expr.MatchString(key)
		}) {
			continue
		}

		typeRep, err := evalTypeRepresentationFromJSONPath(httpSchema, jsonPath, &op, isGlobal)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", key, err)
		}

		if typeRep == nil {
			continue
		}

		httpSchema.Procedures[key] = op
		targets[BuildArgumentPresetJSONPathKey(key, jsonPath)] = typeRep
	}

	return jsonPath, targets, nil
}

// BuildArgumentPresetKey builds the argument preset key.
func BuildArgumentPresetJSONPathKey(operationName string, jsonPath *jsonpath.Path) string {
	return fmt.Sprintf("%s:%s", operationName, jsonPath.String())
}

func evalTypeRepresentationFromJSONPath(
	httpSchema *rest.NDCHttpSchema,
	jsonPath *jsonpath.Path,
	operation *rest.OperationInfo,
	isGlobal bool,
) (schema.TypeRepresentation, error) {
	if len(operation.Arguments) == 0 {
		return nil, nil
	}

	segments := jsonPath.Query().Segments()
	rootSelectorName, ok := segments[0].Selectors()[0].(spec.Name)
	if !ok || rootSelectorName == "" {
		return nil, errors.New("invalid json path. The root selector must be an object name")
	}

	rootSelector := string(rootSelectorName)
	argument, ok := operation.Arguments[rootSelector]
	if !ok {
		return nil, nil
	}

	argumentType, typeRep, err := evalArgumentFromJSONPath(
		httpSchema,
		argument.Type,
		segments[1:],
		[]string{rootSelector},
	)
	if err != nil {
		return nil, err
	}

	if typeRep == nil {
		return nil, nil
	}

	// if the json path selects the root field only, remove the argument field
	if len(segments) == 1 && isGlobal {
		delete(operation.Arguments, rootSelector)
	} else {
		argument.Type = argumentType.Encode()
		operation.Arguments[rootSelector] = argument
	}

	return typeRep, nil
}

func evalArgumentFromJSONPath(
	httpSchema *rest.NDCHttpSchema,
	typeSchema schema.Type,
	segments []*spec.Segment,
	fieldPaths []string,
) (schema.TypeEncoder, schema.TypeRepresentation, error) {
	rawType, err := typeSchema.InterfaceT()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		underlyingType, typeRep, err := evalArgumentFromJSONPath(httpSchema, t.UnderlyingType, segments, fieldPaths)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		if underlyingType == nil {
			return nil, nil, nil
		}

		return utils.WrapNullableTypeEncoder(underlyingType), typeRep, nil
	case *schema.ArrayType:
		if len(segments) == 0 {
			return schema.NewNullableType(t), schema.NewTypeRepresentationJSON().Encode(), nil
		}

		switch s := segments[0].Selectors()[0].(type) {
		case spec.WildcardSelector, spec.Index, spec.SliceSelector:
			newType, typeRep, err := evalArgumentFromJSONPath(httpSchema, t.ElementType, segments[1:], append(fieldPaths, s.String()))
			if err != nil {
				return nil, nil, fmt.Errorf("%s%s: %w", strings.Join(fieldPaths, "."), s.String(), err)
			}

			return schema.NewArrayType(newType), typeRep, nil
		default:
			return nil, nil, nil
		}
	case *schema.NamedType:
		if scalarType, ok := httpSchema.ScalarTypes[t.Name]; ok {
			return schema.NewNullableType(t), scalarType.Representation, nil
		}

		objectType, ok := httpSchema.ObjectTypes[t.Name]
		if !ok {
			return nil, nil, nil
		}

		if len(segments) == 0 {
			return schema.NewNullableType(t), schema.NewTypeRepresentationJSON().Encode(), nil
		}

		selectorName, ok := segments[0].Selectors()[0].(spec.Name)
		if !ok {
			return nil, nil, nil
		}

		if selectorName == "" {
			return nil, nil, fmt.Errorf("invalid json path %s, empty selector name: %s", strings.Join(fieldPaths, "."), segments[0].String())
		}

		selector := string(selectorName)
		field, ok := objectType.Fields[selector]
		if !ok {
			return nil, nil, nil
		}

		newFieldType, typeRep, err := evalArgumentFromJSONPath(httpSchema, field.Type, segments[1:], append(fieldPaths, selector))
		if err != nil {
			return nil, nil, err
		}

		if newFieldType == nil {
			return nil, nil, nil
		}

		field.Type = newFieldType.Encode()
		objectType.Fields[selector] = field
		httpSchema.ObjectTypes[t.Name] = objectType

		return t, typeRep, nil
	default:
		return nil, nil, nil
	}
}
