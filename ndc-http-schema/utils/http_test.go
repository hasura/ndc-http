package utils

import (
	"net/url"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIsContentTypeJSON(t *testing.T) {
	testCases := []struct {
		Name        string
		ContentType string
		Expected    bool
	}{
		{
			Name:        "application_json",
			ContentType: "application/json",
			Expected:    true,
		},
		{
			Name:        "application_json_charset",
			ContentType: "application/json; charset=utf-8",
			Expected:    false, // exact match only in the implementation
		},
		{
			Name:        "custom_json",
			ContentType: "application/vnd.api+json",
			Expected:    true,
		},
		{
			Name:        "text_plain",
			ContentType: "text/plain",
			Expected:    false,
		},
		{
			Name:        "application_xml",
			ContentType: "application/xml",
			Expected:    false,
		},
		{
			Name:        "empty",
			ContentType: "",
			Expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsContentTypeJSON(tc.ContentType)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestIsContentTypeXML(t *testing.T) {
	testCases := []struct {
		Name        string
		ContentType string
		Expected    bool
	}{
		{
			Name:        "application_xml",
			ContentType: "application/xml",
			Expected:    true,
		},
		{
			Name:        "text_xml",
			ContentType: "text/xml",
			Expected:    false, // not ends with +xml
		},
		{
			Name:        "custom_xml",
			ContentType: "application/soap+xml",
			Expected:    true,
		},
		{
			Name:        "application_json",
			ContentType: "application/json",
			Expected:    false,
		},
		{
			Name:        "empty",
			ContentType: "",
			Expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsContentTypeXML(tc.ContentType)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestIsContentTypeText(t *testing.T) {
	testCases := []struct {
		Name        string
		ContentType string
		Expected    bool
	}{
		{
			Name:        "text_plain",
			ContentType: "text/plain",
			Expected:    true,
		},
		{
			Name:        "text_html",
			ContentType: "text/html",
			Expected:    true,
		},
		{
			Name:        "text_csv",
			ContentType: "text/csv",
			Expected:    true,
		},
		{
			Name:        "image_svg",
			ContentType: "image/svg+xml",
			Expected:    true,
		},
		{
			Name:        "image_png",
			ContentType: "image/png",
			Expected:    false,
		},
		{
			Name:        "application_json",
			ContentType: "application/json",
			Expected:    false,
		},
		{
			Name:        "empty",
			ContentType: "",
			Expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsContentTypeText(tc.ContentType)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestIsContentTypeBinary(t *testing.T) {
	testCases := []struct {
		Name        string
		ContentType string
		Expected    bool
	}{
		{
			Name:        "application_octet_stream",
			ContentType: "application/octet-stream",
			Expected:    true,
		},
		{
			Name:        "application_pdf",
			ContentType: "application/pdf",
			Expected:    true,
		},
		{
			Name:        "image_png",
			ContentType: "image/png",
			Expected:    true,
		},
		{
			Name:        "image_jpeg",
			ContentType: "image/jpeg",
			Expected:    true,
		},
		{
			Name:        "video_mp4",
			ContentType: "video/mp4",
			Expected:    true,
		},
		{
			Name:        "text_plain",
			ContentType: "text/plain",
			Expected:    false,
		},
		{
			Name:        "audio_mp3",
			ContentType: "audio/mp3",
			Expected:    false,
		},
		{
			Name:        "empty",
			ContentType: "",
			Expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsContentTypeBinary(tc.ContentType)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestIsContentTypeMultipartForm(t *testing.T) {
	testCases := []struct {
		Name        string
		ContentType string
		Expected    bool
	}{
		{
			Name:        "multipart_form_data",
			ContentType: "multipart/form-data",
			Expected:    true,
		},
		{
			Name:        "multipart_mixed",
			ContentType: "multipart/mixed",
			Expected:    true,
		},
		{
			Name:        "multipart_with_boundary",
			ContentType: "multipart/form-data; boundary=----WebKitFormBoundary",
			Expected:    true,
		},
		{
			Name:        "application_json",
			ContentType: "application/json",
			Expected:    false,
		},
		{
			Name:        "empty",
			ContentType: "",
			Expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := IsContentTypeMultipartForm(tc.ContentType)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestCloneURL(t *testing.T) {
	testCases := []struct {
		Name     string
		InputURL string
	}{
		{
			Name:     "simple_url",
			InputURL: "https://example.com",
		},
		{
			Name:     "url_with_path",
			InputURL: "https://example.com/api/v1/users",
		},
		{
			Name:     "url_with_query",
			InputURL: "https://example.com/search?q=test&limit=10",
		},
		{
			Name:     "url_with_fragment",
			InputURL: "https://example.com/page#section",
		},
		{
			Name:     "url_with_all",
			InputURL: "https://user:pass@example.com:8080/api/v1?key=value#fragment",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			original, err := url.Parse(tc.InputURL)
			assert.NilError(t, err)

			cloned := CloneURL(original)

			// Verify it's a different instance
			assert.Assert(t, original != cloned, "cloned URL should be a different instance")

			// Verify the content is the same
			assert.Equal(t, original.String(), cloned.String())

			// Modify the clone and verify original is unaffected
			cloned.Path = "/modified"
			assert.Assert(t, original.Path != cloned.Path, "modifying clone should not affect original")
		})
	}
}
