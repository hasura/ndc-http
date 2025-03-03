package exhttp

import (
	"log/slog"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHTTPTransport(t *testing.T) {
	_ = HTTPTransportConfig{}.ToTransport()
	_, err := HTTPTransportTLSConfig{}.ToTransport(slog.Default())
	assert.NilError(t, err)
}
