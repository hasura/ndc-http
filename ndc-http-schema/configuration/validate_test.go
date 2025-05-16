package configuration

import (
	"bytes"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gotest.tools/v3/assert"
)

func TestConfigValidator(t *testing.T) {
	testCases := []struct {
		Dir      string
		ErrorMsg string
		IsOk     bool
		HasError bool
	}{
		{
			Dir: "testdata/validation",
		},
		{
			Dir:      "testdata/server_empty",
			ErrorMsg: "failed to build schema files",
		},
	}

	spacesRegex := regexp.MustCompile(`\n(\s|\t)*\n`)

	for _, tc := range testCases {
		t.Run(tc.Dir, func(t *testing.T) {
			connectorDir := filepath.Join(tc.Dir, "connector", "http")
			expectedBytes, err := os.ReadFile(filepath.Join(tc.Dir, "expected.tpl"))
			config, schemas, mergedSchema, err := UpdateHTTPConfiguration(
				connectorDir,
				slog.Default(),
			)
			if tc.ErrorMsg != "" {
				assert.ErrorContains(t, err, tc.ErrorMsg)

				return
			}

			assert.NilError(t, err)

			validStatus, err := ValidateConfiguration(
				config,
				connectorDir,
				schemas,
				mergedSchema,
				slog.Default(),
				true,
			)
			assert.NilError(t, err)

			var buf bytes.Buffer
			validStatus.Render(&buf)
			assert.Equal(t, tc.IsOk, validStatus.IsOk())
			log.Println(validStatus.errors, validStatus.warnings)
			assert.Equal(t, tc.HasError, validStatus.HasError())
			assert.Equal(
				t,
				spacesRegex.ReplaceAllString(string(expectedBytes), "\n"),
				spacesRegex.ReplaceAllString(buf.String(), "\n"),
			)

			// test README.md
			var readmeBuf bytes.Buffer
			readmeBytes, err := os.ReadFile(filepath.Join(tc.Dir, "README.md"))
			assert.NilError(t, err)

			validStatus.renderReadme(&readmeBuf)
			readmeOutput := readmeBuf.String()
			log.Println(readmeOutput)
			assert.Equal(
				t,
				spacesRegex.ReplaceAllString(string(readmeBytes), "\n"),
				spacesRegex.ReplaceAllString(readmeOutput, "\n"),
			)
		})
	}
}
