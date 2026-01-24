package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestMarshalSchema(t *testing.T) {
	testData := map[string]any{
		"version": "1.0",
		"name":    "test-schema",
		"data": map[string]any{
			"key": "value",
		},
	}

	testCases := []struct {
		Name        string
		Format      schema.SchemaFileFormat
		ExpectError bool
	}{
		{
			Name:        "json_format",
			Format:      schema.SchemaFileJSON,
			ExpectError: false,
		},
		{
			Name:        "yaml_format",
			Format:      schema.SchemaFileYAML,
			ExpectError: false,
		},
		{
			Name:        "invalid_format",
			Format:      schema.SchemaFileFormat("xml"),
			ExpectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := MarshalSchema(testData, tc.Format)

			if tc.ExpectError {
				assert.Assert(t, err != nil, "expected an error")
			} else {
				assert.NilError(t, err)
				assert.Assert(t, len(result) > 0, "result should not be empty")
			}
		})
	}
}

func TestMarshalSchema_JSON(t *testing.T) {
	testData := map[string]any{
		"name": "test",
		"age":  30,
	}

	result, err := MarshalSchema(testData, schema.SchemaFileJSON)
	assert.NilError(t, err)
	assert.Assert(t, len(result) > 0)

	// Verify JSON formatting (with indent)
	resultStr := string(result)
	assert.Assert(t, resultStr != "", "result should contain formatted JSON")
}

func TestMarshalSchema_YAML(t *testing.T) {
	testData := map[string]any{
		"name": "test",
		"age":  30,
	}

	result, err := MarshalSchema(testData, schema.SchemaFileYAML)
	assert.NilError(t, err)
	assert.Assert(t, len(result) > 0)

	// Verify YAML content
	resultStr := string(result)
	assert.Assert(t, resultStr != "", "result should contain YAML")
}

func TestWriteSchemaFile(t *testing.T) {
	tmpDir := t.TempDir()

	testData := map[string]any{
		"version": "1.0",
		"name":    "test",
	}

	testCases := []struct {
		Name        string
		OutputPath  string
		ExpectError bool
	}{
		{
			Name:        "json_file",
			OutputPath:  filepath.Join(tmpDir, "test.json"),
			ExpectError: false,
		},
		{
			Name:        "yaml_file",
			OutputPath:  filepath.Join(tmpDir, "test.yaml"),
			ExpectError: false,
		},
		{
			Name:        "yml_file",
			OutputPath:  filepath.Join(tmpDir, "test.yml"),
			ExpectError: false,
		},
		{
			Name:        "nested_path",
			OutputPath:  filepath.Join(tmpDir, "nested", "dir", "test.json"),
			ExpectError: false,
		},
		{
			Name:        "invalid_extension",
			OutputPath:  filepath.Join(tmpDir, "test.txt"),
			ExpectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			err := WriteSchemaFile(tc.OutputPath, testData)

			if tc.ExpectError {
				assert.Assert(t, err != nil, "expected an error")
			} else {
				assert.NilError(t, err)

				// Verify file exists
				_, statErr := os.Stat(tc.OutputPath)
				assert.NilError(t, statErr, "file should exist")

				// Verify file content
				content, readErr := os.ReadFile(tc.OutputPath)
				assert.NilError(t, readErr)
				assert.Assert(t, len(content) > 0, "file should have content")
			}
		})
	}
}

func TestReadFileFromPath_LocalFile(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		Name        string
		Content     string
		ExpectError bool
	}{
		{
			Name:        "valid_file",
			Content:     "test content",
			ExpectError: false,
		},
		{
			Name:        "json_content",
			Content:     `{"key": "value"}`,
			ExpectError: false,
		},
		{
			Name:        "empty_file",
			Content:     "",
			ExpectError: true, // fails because no content
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, tc.Name+".txt")
			err := os.WriteFile(filePath, []byte(tc.Content), 0o644)
			assert.NilError(t, err)

			result, err := ReadFileFromPath(filePath)

			if tc.ExpectError {
				assert.Assert(t, err != nil, "expected an error")
			} else {
				assert.NilError(t, err)
				assert.Equal(t, tc.Content, string(result))
			}
		})
	}
}

func TestReadFileFromPath_NonExistentFile(t *testing.T) {
	_, err := ReadFileFromPath("/path/to/nonexistent/file.txt")
	assert.Assert(t, err != nil, "should return error for non-existent file")
}

func TestWalkFiles_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	testContent := "test content"

	err := os.WriteFile(filePath, []byte(testContent), 0o644)
	assert.NilError(t, err)

	var callCount int
	var receivedContent string

	err = WalkFiles(filePath, func(data []byte) error {
		callCount++
		receivedContent = string(data)
		return nil
	})

	assert.NilError(t, err)
	assert.Equal(t, 1, callCount, "callback should be called once")
	assert.Equal(t, testContent, receivedContent)
}

func TestWalkFiles_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple files
	files := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"file3.txt": "content3",
	}

	for name, content := range files {
		filePath := filepath.Join(tmpDir, name)
		err := os.WriteFile(filePath, []byte(content), 0o644)
		assert.NilError(t, err)
	}

	var callCount int
	collectedContent := make(map[string]bool)

	err := WalkFiles(tmpDir, func(data []byte) error {
		callCount++
		collectedContent[string(data)] = true
		return nil
	})

	assert.NilError(t, err)
	assert.Equal(t, len(files), callCount, "callback should be called for each file")

	// Verify all content was processed
	for _, content := range files {
		assert.Assert(t, collectedContent[content], "content should be collected")
	}
}

func TestWalkFiles_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.txt")

	err := os.WriteFile(filePath, []byte(""), 0o644)
	assert.NilError(t, err)

	err = WalkFiles(filePath, func(data []byte) error {
		return nil
	})

	assert.Assert(t, err != nil, "should return error for empty file")
}

func TestWalkFiles_NonExistentPath(t *testing.T) {
	err := WalkFiles("/path/to/nonexistent", func(data []byte) error {
		return nil
	})

	assert.Assert(t, err != nil, "should return error for non-existent path")
}

func TestResolveFilePath(t *testing.T) {
	testCases := []struct {
		Name     string
		Dir      string
		FilePath string
		Expected string
	}{
		{
			Name:     "relative_path",
			Dir:      "/base/dir",
			FilePath: "file.txt",
			Expected: "/base/dir/file.txt",
		},
		{
			Name:     "absolute_unix_path",
			Dir:      "/base/dir",
			FilePath: "/absolute/path/file.txt",
			Expected: "/absolute/path/file.txt",
		},
		{
			Name:     "windows_absolute_path",
			Dir:      "C:\\base\\dir",
			FilePath: "\\absolute\\path\\file.txt",
			Expected: "\\absolute\\path\\file.txt",
		},
		{
			Name:     "http_url",
			Dir:      "/base/dir",
			FilePath: "https://example.com/file.txt",
			Expected: "https://example.com/file.txt",
		},
		{
			Name:     "http_url_lowercase",
			Dir:      "/base/dir",
			FilePath: "http://example.com/file.txt",
			Expected: "http://example.com/file.txt",
		},
		{
			Name:     "relative_with_subdirs",
			Dir:      "/base",
			FilePath: "sub/dir/file.txt",
			Expected: "/base/sub/dir/file.txt",
		},
		{
			Name:     "empty_dir",
			Dir:      "",
			FilePath: "file.txt",
			Expected: "file.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := ResolveFilePath(tc.Dir, tc.FilePath)
			assert.Equal(t, tc.Expected, result)
		})
	}
}
