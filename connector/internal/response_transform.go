package internal

import (
	"fmt"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/utils"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

func (client *HTTPClient) transformResponse(body any) (any, error) {
	if client.requests == nil || client.requests.Schema == nil ||
		client.requests.Schema.NDCHttpSchema == nil ||
		client.requests.Schema.Settings == nil ||
		len(client.requests.Schema.Settings.ResponseTransforms) == 0 {
		return body, nil
	}

	var err error

	for _, setting := range client.requests.Schema.Settings.ResponseTransforms {
		if len(setting.Targets) > 0 &&
			!slices.Contains(setting.Targets, client.requests.OperationName) {
			continue
		}

		body, err = NewResponseTransformer(setting, false).Transform(body)
		if err != nil {
			return nil, err
		}
	}

	return body, nil
}

// ResponseTransformer is a processor to transform the response body from a template.
type ResponseTransformer struct {
	setting rest.ResponseTransformSetting
	strict  bool
}

// NewResponseTransformer creates a ResponseTransformer instance.
func NewResponseTransformer(
	setting rest.ResponseTransformSetting,
	strict bool,
) *ResponseTransformer {
	return &ResponseTransformer{
		setting: setting,
		strict:  strict,
	}
}

// Transform evaluates and transform the response body.
func (rt *ResponseTransformer) Transform(responseBody any) (any, error) {
	return rt.evalResultType(rt.setting.Body, responseBody, []string{})
}

func (rt *ResponseTransformer) evalResultType(
	transformValue any,
	responseBody any,
	fieldPaths []string,
) (any, error) {
	switch value := transformValue.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, *bool, *int, *int8, *int16, *int32, *int64, *uint, *uint8, *uint16, *uint32, *uint64, *float32, *float64:
		return value, nil
	case string:
		return rt.evalStringValue(value, responseBody, fieldPaths)
	case *string:
		if value == nil {
			return nil, nil
		}

		return rt.evalStringValue(*value, responseBody, fieldPaths)
	case []any:
		if len(value) == 0 {
			return value, nil
		}

		result := make([]any, len(value))

		for i, elem := range value {
			elemValue, err := rt.evalResultType(elem, responseBody, append(fieldPaths, fmt.Sprintf("[%d]", i)))
			if err != nil {
				return nil, err
			}

			result[i] = elemValue
		}

		return result, nil
	case map[string]any:
		if len(value) == 0 {
			return value, nil
		}

		result := make(map[string]any)

		for key, elem := range value {
			elemValue, err := rt.evalResultType(elem, responseBody, append(fieldPaths, key))
			if err != nil {
				return nil, err
			}

			result[key] = elemValue
		}

		return result, nil
	default:
		return nil, fmt.Errorf("%s: failed to transform value: %v", strings.Join(fieldPaths, "."), transformValue)
	}
}

func (rt *ResponseTransformer) evalStringValue(
	transformValue string,
	responseBody any,
	fieldPaths []string,
) (any, error) {
	selector, err := jsonpath.Parse(transformValue)
	if err != nil {
		return transformValue, nil //nolint:nilerr
	}

	return rt.evalJSONPath(responseBody, selector.Query().Segments(), fieldPaths)
}

func (rt *ResponseTransformer) evalJSONPath(
	value any,
	segments []*spec.Segment,
	fieldPaths []string,
) (any, error) {
	if len(segments) == 0 || len(segments[0].Selectors()) == 0 {
		return value, nil
	}

	rawSelector := segments[0].Selectors()[0]

	switch selector := rawSelector.(type) {
	case spec.Name:
		return rt.evalNameSelector(value, segments, fieldPaths, selector)
	case spec.WildcardSelector:
		return rt.evalWildCardSelector(value, segments, fieldPaths)
	case spec.SliceSelector:
		return rt.evalSliceSelector(value, segments, fieldPaths, selector)
	case spec.Index:
		return rt.evalIndexSelector(value, segments, fieldPaths, selector)
	default:
		return nil, nil
	}
}

func (rt *ResponseTransformer) evalNameSelector(
	value any,
	segments []*spec.Segment,
	fieldPaths []string,
	selector spec.Name,
) (any, error) {
	valueMap, ok := value.(map[string]any)
	if !ok {
		if rt.strict {
			return nil, fmt.Errorf(
				"failed to select json path at %s; expected object, got: %v",
				strings.Join(fieldPaths, "."),
				value,
			)
		}

		return nil, nil
	}

	if len(valueMap) == 0 {
		return nil, nil
	}

	selectorStr := string(selector)

	result, ok := valueMap[selectorStr]
	if !ok {
		if rt.strict {
			return nil, fmt.Errorf(
				"failed to select json path at %s; value at %s does not exist",
				strings.Join(fieldPaths, "."),
				selectorStr,
			)
		}

		return nil, nil
	}

	if len(segments) == 1 {
		return result, nil
	}

	return rt.evalJSONPath(result, segments[1:], append(fieldPaths, selectorStr))
}

func (rt *ResponseTransformer) evalWildCardSelector(
	value any,
	segments []*spec.Segment,
	fieldPaths []string,
) (any, error) {
	values, sok := value.([]any)
	if !sok {
		if rt.strict {
			return nil, fmt.Errorf(
				"failed to select json path at %s; expected array, got: %v",
				strings.Join(fieldPaths, "."),
				value,
			)
		}

		return nil, nil
	}

	if values == nil {
		return nil, nil
	}

	newValues := []any{}

	for i, elem := range values {
		newElem, err := rt.evalJSONPath(
			elem,
			segments[1:],
			append(fieldPaths, fmt.Sprintf("[%d]", i)),
		)
		if err != nil {
			return nil, err
		}

		if utils.IsNil(newElem) {
			continue
		}

		newValues = append(newValues, newElem)
	}

	return newValues, nil
}

func (rt *ResponseTransformer) evalSliceSelector(
	value any,
	segments []*spec.Segment,
	fieldPaths []string,
	selector spec.SliceSelector,
) (any, error) {
	values, sok := value.([]any)
	if !sok {
		if rt.strict {
			return nil, fmt.Errorf(
				"failed to select json path at %s; expected array, got: %v",
				strings.Join(fieldPaths, "."),
				value,
			)
		}

		return nil, nil
	}

	if values == nil {
		return values, nil
	}

	step := selector.Step()
	step = max(step, 1)

	end := selector.End()
	if end >= len(values) {
		end = len(values) - 1
	}

	var newValues []any

	for i := selector.Start(); i <= end; i += step {
		newElem, err := rt.evalJSONPath(
			values[i],
			segments[1:],
			append(fieldPaths, fmt.Sprintf("[%d]", i)),
		)
		if err != nil {
			return nil, err
		}

		newValues = append(newValues, newElem)
	}

	return newValues, nil
}

func (rt *ResponseTransformer) evalIndexSelector(
	value any,
	segments []*spec.Segment,
	fieldPaths []string,
	selector spec.Index,
) (any, error) {
	index := int(selector)

	values, sok := value.([]any)
	if !sok {
		if rt.strict {
			return nil, fmt.Errorf(
				"failed to select json path at %s; expected array, got: %v",
				strings.Join(fieldPaths, "."),
				value,
			)
		}

		return nil, nil
	}

	if len(values) <= index {
		return nil, nil
	}

	newValue, err := rt.evalJSONPath(
		values[index],
		segments[1:],
		append(fieldPaths, fmt.Sprintf("[%d]", index)),
	)
	if err != nil {
		return nil, err
	}

	return newValue, nil
}
