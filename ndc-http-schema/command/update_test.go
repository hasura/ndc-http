package command

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"gotest.tools/v3/assert"
)

func TestUpdateCommand(t *testing.T) {
	testCases := []struct {
		Argument UpdateCommandArguments
	}{
		// go run ./ndc-http-schema update -d ./ndc-http-schema/command/testdata/patch
		{
			Argument: UpdateCommandArguments{
				Dir: "testdata/patch",
			},
		},
		// go run ./ndc-http-schema update -d ./ndc-http-schema/command/testdata/auth
		{
			Argument: UpdateCommandArguments{
				Dir: "testdata/auth",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Argument.Dir, func(t *testing.T) {
			assert.NilError(t, UpdateConfiguration(&tc.Argument, slog.Default(), true))

			output := readRuntimeSchemaFile(t, tc.Argument.Dir+"/schema.output.json")
			expected := readRuntimeSchemaFile(t, tc.Argument.Dir+"/expected.json")
			assert.DeepEqual(t, expected, output)
		})
	}
}

func readRuntimeSchemaFile(t *testing.T, filePath string) []configuration.NDCHttpRuntimeSchema {
	t.Helper()
	rawBytes, err := os.ReadFile(filePath)
	assert.NilError(t, err)

	var result []configuration.NDCHttpRuntimeSchema
	assert.NilError(t, json.Unmarshal(rawBytes, &result))

	return result
}
