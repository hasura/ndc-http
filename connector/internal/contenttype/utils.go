package contenttype

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

var (
	errArgumentRequired        = errors.New("argument is required")
	errRequestBodyTypeRequired = errors.New("failed to decode request body, empty body type")
)

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// StringifySimpleScalar converts a simple scalar value to string.
func StringifySimpleScalar(val reflect.Value, kind reflect.Kind) (string, error) {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(val.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(val.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'g', -1, val.Type().Bits()), nil
	case reflect.String:
		return val.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(val.Bool()), nil
	case reflect.Interface:
		return fmt.Sprint(val.Interface()), nil
	default:
		value := val.Interface()
		if stringer, ok := value.(fmt.Stringer); ok {
			return stringer.String(), nil
		}

		j, err := json.Marshal(value)
		if err != nil {
			return "", err
		}

		return string(j), nil
	}
}

func evalObjectJSONValue(value any, stringifyJSON bool) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if ok || !stringifyJSON {
		return object, ok
	}

	str, ok := value.(string)
	if !ok || str == "" {
		return nil, false
	}

	var obj map[string]any

	err := json.Unmarshal([]byte(str), &obj)
	if err != nil {
		return nil, false
	}

	return obj, true
}
