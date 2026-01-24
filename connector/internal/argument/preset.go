package argument

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// ArgumentPreset represents an argument preset.
type ArgumentPreset struct {
	Path    *jsonpath.Path
	Value   ArgumentPresetValueGetter
	Targets map[string]schema.TypeRepresentation
}

// NewArgumentPreset create a new ArgumentPreset instance.
func NewArgumentPreset(
	httpSchema *rest.NDCHttpSchema,
	preset rest.ArgumentPresetConfig,
	isGlobal bool,
) (*ArgumentPreset, error) {
	jsonPath, targets, err := configuration.ValidateArgumentPreset(httpSchema, preset, isGlobal)
	if err != nil {
		return nil, err
	}

	getter, err := NewArgumentPresetValueGetter(preset.Value)
	if err != nil {
		return nil, err
	}

	return &ArgumentPreset{
		Path:    jsonPath,
		Targets: targets,
		Value:   getter,
	}, nil
}

// Evaluate iterates and inject values into request arguments recursively.
func (ap ArgumentPreset) Evaluate(
	operationName string,
	arguments map[string]any,
	headers map[string]string,
) (map[string]any, error) {
	key := configuration.BuildArgumentPresetJSONPathKey(operationName, ap.Path)
	if _, ok := ap.Targets[key]; !ok {
		return arguments, nil
	}

	segments := ap.Path.Query().Segments()

	rootSelector, ok := segments[0].Selectors()[0].(spec.Name)
	if !ok || rootSelector == "" {
		return nil, errors.New("invalid json path. The root selector must be an object name")
	}

	value, err := ap.Value.GetValue(headers, ap.getTypeRepresentation(key))
	if err != nil {
		return nil, err
	}

	selectorStr := string(rootSelector)

	if len(segments) == 1 {
		arguments[selectorStr] = value

		return arguments, nil
	}

	nestedValue, err := ap.evalNestedField(
		segments[1:],
		arguments[string(rootSelector)],
		value,
		[]string{selectorStr},
	)
	if err != nil {
		return nil, err
	}

	arguments[selectorStr] = nestedValue

	return arguments, nil
}

func (ap ArgumentPreset) evalNestedField(
	segments []*spec.Segment,
	argument any,
	value any,
	fieldPaths []string,
) (any, error) {
	segmentsLen := len(segments)
	if segmentsLen == 0 || len(segments[0].Selectors()) == 0 {
		return value, nil
	}

	switch selector := segments[0].Selectors()[0].(type) {
	case spec.Name:
		argumentMap, mok := argument.(map[string]any)
		if !mok {
			argumentMap = make(map[string]any)
		}

		selectorStr := string(selector)

		if segmentsLen == 1 {
			argumentMap[selectorStr] = value

			return argumentMap, nil
		}

		nestedValue, err := ap.evalNestedField(
			segments[1:],
			argumentMap[selectorStr],
			value,
			append(fieldPaths, selectorStr),
		)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		argumentMap[selectorStr] = nestedValue

		return argumentMap, nil
	case spec.WildcardSelector:
		argumentSlice, sok := argument.([]any)
		if !sok {
			return argument, nil
		}

		for i, arg := range argumentSlice {
			var err error

			argumentSlice[i], err = ap.evalNestedField(
				segments[1:],
				arg,
				value,
				append(fieldPaths, strconv.Itoa(i)),
			)
			if err != nil {
				return nil, err
			}
		}

		return argumentSlice, nil
	case spec.SliceSelector:
		argumentSlice, sok := argument.([]any)
		if !sok {
			return argument, nil
		}

		step := selector.Step()
		step = max(step, 1)

		end := selector.End()
		if end >= len(argumentSlice) {
			end = len(argumentSlice) - 1
		}

		for i := selector.Start(); i <= end; i += step {
			var err error

			argumentSlice[i], err = ap.evalNestedField(
				segments[1:],
				argumentSlice[i],
				value,
				append(fieldPaths, strconv.Itoa(i)),
			)
			if err != nil {
				return nil, err
			}
		}

		return argumentSlice, nil
	case spec.Index:
		index := int(selector)
		argumentSlice, sok := argument.([]any)

		if !sok || len(argumentSlice) <= index {
			return argument, nil
		}

		newValue, err := ap.evalNestedField(
			segments[1:],
			argumentSlice[index],
			value,
			append(fieldPaths, strconv.Itoa(index)),
		)
		if err != nil {
			return nil, err
		}

		argumentSlice[index] = newValue

		return argumentSlice, nil
	default:
		return argument, nil
	}
}

func (ap ArgumentPreset) getTypeRepresentation(key string) schema.TypeRepresentation {
	if rep, ok := ap.Targets[key]; ok {
		return rep
	}

	return nil
}
