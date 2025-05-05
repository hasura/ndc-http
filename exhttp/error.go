package exhttp

import (
	"fmt"
	"io"
	"net/http"
)

// HTTPError represents an error from HTTP response.
type HTTPError struct {
	StatusCode int         `json:"statusCode"`
	Headers    http.Header `json:"headers"`
	Body       []byte      `json:"body"`
}

// HTTPErrorFromResponse creates an error from the HTTP response.
func HTTPErrorFromResponse(res *http.Response) error {
	result := &HTTPError{
		StatusCode: res.StatusCode,
		Headers:    res.Header,
	}

	if res.Body != nil {
		rawBody, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()

		if readErr == nil {
			result.Body = rawBody
		}
	}

	return result
}

// Error implements the error interface.
func (he HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", he.StatusCode, string(he.Body))
}
