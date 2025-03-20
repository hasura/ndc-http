package security

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hasura/ndc-http/exhttp"
)

// NewHTTPClientTLS creates a new HTTP Client with TLS configuration.
func NewHTTPClientTLS(baseClient *http.Client, tlsConfig *exhttp.TLSConfig, logger *slog.Logger) (*http.Client, error) {
	transport, err := exhttp.NewTLSTransport(baseClient.Transport, tlsConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	return &http.Client{
		Transport:     transport,
		CheckRedirect: baseClient.CheckRedirect,
		Jar:           baseClient.Jar,
		Timeout:       baseClient.Timeout,
	}, nil
}
