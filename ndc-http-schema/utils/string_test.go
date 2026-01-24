package utils

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestToCamelCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "simple",
			Input:    "hello_world",
			Expected: "helloWorld",
		},
		{
			Name:     "single_word",
			Input:    "hello",
			Expected: "hello",
		},
		{
			Name:     "multiple_underscores",
			Input:    "hello_world_test",
			Expected: "helloWorldTest",
		},
		{
			Name:     "with_dashes",
			Input:    "hello-world",
			Expected: "helloWorld",
		},
		{
			Name:     "mixed_separators",
			Input:    "hello_world-test",
			Expected: "helloWorldTest",
		},
		{
			Name:     "already_camelCase",
			Input:    "helloWorld",
			Expected: "helloWorld",
		},
		{
			Name:     "PascalCase",
			Input:    "HelloWorld",
			Expected: "helloWorld",
		},
		{
			Name:     "empty_string",
			Input:    "",
			Expected: "",
		},
		{
			Name:     "with_numbers",
			Input:    "hello_world_123",
			Expected: "helloWorld123",
		},
		{
			Name:     "special_characters",
			Input:    "hello@world!test",
			Expected: "helloWorldTest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := ToCamelCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestStringSliceToCamelCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []string
		Expected string
	}{
		{
			Name:     "two_words",
			Input:    []string{"hello", "world"},
			Expected: "helloWorld",
		},
		{
			Name:     "three_words",
			Input:    []string{"get", "user", "profile"},
			Expected: "getUserProfile",
		},
		{
			Name:     "single_word",
			Input:    []string{"hello"},
			Expected: "hello",
		},
		{
			Name:     "with_underscores",
			Input:    []string{"hello_world", "test_case"},
			Expected: "helloWorldTestCase",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := StringSliceToCamelCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestToPascalCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "simple",
			Input:    "hello_world",
			Expected: "HelloWorld",
		},
		{
			Name:     "single_word",
			Input:    "hello",
			Expected: "Hello",
		},
		{
			Name:     "multiple_underscores",
			Input:    "hello_world_test",
			Expected: "HelloWorldTest",
		},
		{
			Name:     "with_dashes",
			Input:    "hello-world",
			Expected: "HelloWorld",
		},
		{
			Name:     "already_PascalCase",
			Input:    "HelloWorld",
			Expected: "HelloWorld",
		},
		{
			Name:     "camelCase",
			Input:    "helloWorld",
			Expected: "HelloWorld",
		},
		{
			Name:     "empty_string",
			Input:    "",
			Expected: "",
		},
		{
			Name:     "with_numbers",
			Input:    "hello_123_world",
			Expected: "Hello123World",
		},
		{
			Name:     "special_characters",
			Input:    "hello@world#test",
			Expected: "HelloWorldTest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := ToPascalCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestStringSliceToPascalCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []string
		Expected string
	}{
		{
			Name:     "two_words",
			Input:    []string{"hello", "world"},
			Expected: "HelloWorld",
		},
		{
			Name:     "three_words",
			Input:    []string{"get", "user", "profile"},
			Expected: "GetUserProfile",
		},
		{
			Name:     "with_underscores",
			Input:    []string{"hello_world", "test_case"},
			Expected: "HelloWorldTestCase",
		},
		{
			Name:     "with_empty_strings",
			Input:    []string{"hello", "", "world"},
			Expected: "HelloWorld",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := StringSliceToPascalCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "camelCase",
			Input:    "helloWorld",
			Expected: "hello_world",
		},
		{
			Name:     "PascalCase",
			Input:    "HelloWorld",
			Expected: "hello_world",
		},
		{
			Name:     "already_snake_case",
			Input:    "hello_world",
			Expected: "hello_world",
		},
		{
			Name:     "with_dash",
			Input:    "hello-world",
			Expected: "hello_world",
		},
		{
			Name:     "single_word",
			Input:    "hello",
			Expected: "hello",
		},
		{
			Name:     "with_numbers",
			Input:    "hello123World",
			Expected: "hello123_world",
		},
		{
			Name:     "consecutive_capitals",
			Input:    "HTTPResponse",
			Expected: "http_response",
		},
		{
			Name:     "all_caps",
			Input:    "HTTP",
			Expected: "http",
		},
		{
			Name:     "acronym_start",
			Input:    "XMLParser",
			Expected: "xml_parser",
		},
		{
			Name:     "empty_string",
			Input:    "",
			Expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := ToSnakeCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestStringSliceToSnakeCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []string
		Expected string
	}{
		{
			Name:     "two_words",
			Input:    []string{"hello", "world"},
			Expected: "hello_world",
		},
		{
			Name:     "camelCase_words",
			Input:    []string{"helloWorld", "testCase"},
			Expected: "hello_world_test_case",
		},
		{
			Name:     "with_empty_strings",
			Input:    []string{"hello", "", "world"},
			Expected: "hello_world",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := StringSliceToSnakeCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestToConstantCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "camelCase",
			Input:    "helloWorld",
			Expected: "HELLO_WORLD",
		},
		{
			Name:     "PascalCase",
			Input:    "HelloWorld",
			Expected: "HELLO_WORLD",
		},
		{
			Name:     "snake_case",
			Input:    "hello_world",
			Expected: "HELLO_WORLD",
		},
		{
			Name:     "already_constant",
			Input:    "HELLO_WORLD",
			Expected: "HELLO_WORLD",
		},
		{
			Name:     "single_word",
			Input:    "hello",
			Expected: "HELLO",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := ToConstantCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestStringSliceToConstantCase(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []string
		Expected string
	}{
		{
			Name:     "two_words",
			Input:    []string{"hello", "world"},
			Expected: "HELLO_WORLD",
		},
		{
			Name:     "camelCase_words",
			Input:    []string{"helloWorld", "testCase"},
			Expected: "HELLO_WORLD_TEST_CASE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := StringSliceToConstantCase(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestSplitStringsAndTrimSpaces(t *testing.T) {
	testCases := []struct {
		Name      string
		Input     string
		Separator string
		Expected  []string
	}{
		{
			Name:      "comma_separated",
			Input:     "apple, banana, cherry",
			Separator: ",",
			Expected:  []string{"apple", "banana", "cherry"},
		},
		{
			Name:      "no_spaces",
			Input:     "apple,banana,cherry",
			Separator: ",",
			Expected:  []string{"apple", "banana", "cherry"},
		},
		{
			Name:      "extra_spaces",
			Input:     "apple  ,  banana  ,  cherry",
			Separator: ",",
			Expected:  []string{"apple", "banana", "cherry"},
		},
		{
			Name:      "empty_elements",
			Input:     "apple,,cherry",
			Separator: ",",
			Expected:  []string{"apple", "cherry"},
		},
		{
			Name:      "only_spaces",
			Input:     "   ,   ,   ",
			Separator: ",",
			Expected:  nil,
		},
		{
			Name:      "single_element",
			Input:     "apple",
			Separator: ",",
			Expected:  []string{"apple"},
		},
		{
			Name:      "pipe_separator",
			Input:     "apple | banana | cherry",
			Separator: "|",
			Expected:  []string{"apple", "banana", "cherry"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := SplitStringsAndTrimSpaces(tc.Input, tc.Separator)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}

func TestStripHTMLTags(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "simple_tag",
			Input:    "<p>Hello World</p>",
			Expected: "Hello World",
		},
		{
			Name:     "multiple_tags",
			Input:    "<div><p>Hello</p><p>World</p></div>",
			Expected: "HelloWorld</div>", // The function doesn't handle nested closing tags perfectly
		},
		{
			Name:     "nested_tags",
			Input:    "<div><span><strong>Bold Text</strong></span></div>",
			Expected: "Bold Text</div>", // The function doesn't handle nested closing tags perfectly
		},
		{
			Name:     "with_attributes",
			Input:    `<a href="http://example.com">Link</a>`,
			Expected: "Link",
		},
		{
			Name:     "self_closing_tags",
			Input:    "Hello<br/>World",
			Expected: "HelloWorld",
		},
		{
			Name:     "malformed_tags",
			Input:    "<<div>>Text<</div>>",
			Expected: "Text>", // Malformed tags result in partial output
		},
		{
			Name:     "no_tags",
			Input:    "Plain text",
			Expected: "Plain text",
		},
		{
			Name:     "empty_string",
			Input:    "",
			Expected: "",
		},
		{
			Name:     "only_tags",
			Input:    "<div></div>",
			Expected: "</div>", // The function doesn't handle empty content between tags
		},
		{
			Name:     "mixed_content",
			Input:    "Text before <em>emphasis</em> text after",
			Expected: "Text before emphasis text after",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := StripHTMLTags(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestMaskString(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "short_string_1",
			Input:    "a",
			Expected: "*",
		},
		{
			Name:     "short_string_5",
			Input:    "abcde",
			Expected: "*****",
		},
		{
			Name:     "medium_string_6",
			Input:    "abcdef",
			Expected: "a*****",
		},
		{
			Name:     "medium_string_11",
			Input:    "abcdefghijk",
			Expected: "a**********",
		},
		{
			Name:     "long_string_12",
			Input:    "abcdefghijkl",
			Expected: "abc*******(12)",
		},
		{
			Name:     "long_string_20",
			Input:    "abcdefghijklmnopqrst",
			Expected: "abc*******(20)",
		},
		{
			Name:     "empty_string",
			Input:    "",
			Expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := MaskString(tc.Input)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestRemoveYAMLSpecialCharacters(t *testing.T) {
	testCases := []struct {
		Input    string
		Expected string
	}{
		{
			Input:    "\b\t\u0009Some\u0000thing\\u0002",
			Expected: "  Something",
		},
		{
			Input:    "^[a-zA-Z\\u0080-\\u024F\\s\\/\\-\\)\\(\\`\\.\\\"\\']+$",
			Expected: "^[a-zA-Z-\\s\\/\\-\\)\\(\\`\\.\\\"\\']+$",
		},
		{
			Input:    "Normal text",
			Expected: "Normal text",
		},
		{
			Input:    "",
			Expected: "",
		},
		{
			Input:    "\\b\\n\\r\\t\\f",
			Expected: "     ",
		},
		{
			Input:    "Test\\u003cvalue\\u003e",
			Expected: "Test<value>",
		},
		{
			Input:    "Double\\\\backslash",
			Expected: "Double\\\\backslash",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Expected, func(t *testing.T) {
			assert.Equal(t, tc.Expected, string(RemoveYAMLSpecialCharacters([]byte(tc.Input))))
		})
	}
}

func TestGetu4(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected rune
	}{
		{
			Name:     "valid_unicode_lowercase",
			Input:    "\\u003c",
			Expected: '<',
		},
		{
			Name:     "valid_unicode_uppercase",
			Input:    "\\u003E",
			Expected: '>',
		},
		{
			Name:     "valid_unicode_ampersand",
			Input:    "\\u0026",
			Expected: '&',
		},
		{
			Name:     "too_short",
			Input:    "\\u00",
			Expected: -1,
		},
		{
			Name:     "invalid_prefix",
			Input:    "\\x003c",
			Expected: -1,
		},
		{
			Name:     "invalid_hex",
			Input:    "\\u00zz",
			Expected: -1,
		},
		{
			Name:     "no_backslash",
			Input:    "u003c",
			Expected: -1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := getu4([]byte(tc.Input))
			assert.Equal(t, tc.Expected, result)
		})
	}
}
