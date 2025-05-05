package internal

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/hasura/ndc-http/exhttp"
	"go.opentelemetry.io/otel/attribute"
)

// IsSensitiveHeader checks if the header name is sensitive.
func IsSensitiveHeader(name string) bool {
	return sensitiveHeaderRegex.MatchString(strings.ToLower(name))
}

func WrapTelemetryTransport(baseTransport *http.Transport, logger *slog.Logger) http.RoundTripper {
	return exhttp.NewTelemetryTransport(baseTransport, exhttp.TelemetryConfig{
		Tracer:     tracer,
		Logger:     logger,
		Attributes: []attribute.KeyValue{attribute.String("db.system", "http")},
	})
}

func evalAcceptContentType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image/*"
	case strings.HasPrefix(contentType, "video/"):
		return "video/*"
	default:
		return contentType
	}
}

func evalForwardedHeaders(req *RetryableRequest, headers map[string]string) {
	for key, value := range headers {
		if req.Headers.Get(key) != "" {
			continue
		}
		req.Headers.Set(key, value)
	}
}
