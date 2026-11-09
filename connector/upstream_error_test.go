package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hasura/ndc-sdk-go/v2/connector"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"gotest.tools/v3/assert"
)

func TestHTTPConnectorUpstreamErrors(t *testing.T) {
	for _, retry := range []struct {
		name   string
		config string
		times  int32
	}{
		{name: "omitted"},
		{name: "zero", config: "    retry:\n      times:\n        value: 0\n"},
		{
			name:   "enabled",
			config: "    retry:\n      times:\n        value: 1\n      delay:\n        value: 1\n",
			times:  1,
		},
	} {
		t.Run(retry.name, func(t *testing.T) {
			var statusCode int
			var contentType, responseBody string
			var attempts atomic.Int32

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(responseBody))
			}))
			defer upstream.Close()

			configDir := t.TempDir()
			config := "files:\n  - file: openapi.json\n    spec: openapi3\n" + retry.config
			err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(config), 0o600)
			assert.NilError(t, err)

			paths := map[string]any{}
			for _, shape := range []string{"object", "array"} {
				resultSchema := map[string]any{
					"type":       "object",
					"properties": map[string]any{"name": map[string]any{"type": "string"}},
				}
				if shape == "array" {
					resultSchema = map[string]any{"type": "array", "items": resultSchema}
				}

				methods := map[string]any{}
				for _, method := range []string{"get", "post"} {
					methods[method] = map[string]any{
						"operationId": method + shape,
						"responses": map[string]any{
							"200": map[string]any{
								"description": "Success",
								"content": map[string]any{
									"application/json": map[string]any{"schema": resultSchema},
								},
							},
						},
					}
				}
				paths["/"+shape] = methods
			}

			spec, err := json.Marshal(map[string]any{
				"openapi":    "3.0.3",
				"info":       map[string]any{"title": "Upstream errors", "version": "1.0"},
				"servers":    []map[string]string{{"url": upstream.URL}},
				"paths":      paths,
				"components": map[string]any{},
			})
			assert.NilError(t, err)
			err = os.WriteFile(filepath.Join(configDir, "openapi.json"), spec, 0o600)
			assert.NilError(t, err)

			connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
				Configuration: configDir,
			}, connector.WithoutRecovery())
			assert.NilError(t, err)
			testServer := connServer.BuildTestServer()
			defer testServer.Close()

			for _, operation := range []string{"getobject", "getarray", "postobject", "postarray"} {
				t.Run(operation, func(t *testing.T) {
					for _, response := range []struct {
						name        string
						status      int
						contentType string
						body        string
						detail      any
					}{
						{
							name:        "unauthorized",
							status:      http.StatusUnauthorized,
							contentType: "application/json",
							body:        `{"error":"unauthorized","message":"Invalid or inactive API key"}`,
							detail: map[string]any{
								"error":   "unauthorized",
								"message": "Invalid or inactive API key",
							},
						},
						{
							name:        "server_error",
							status:      http.StatusInternalServerError,
							contentType: "application/json",
							body:        `{"error":"upstream failed"}`,
							detail:      map[string]any{"error": "upstream failed"},
						},
						{
							name:        "plain_text_error",
							status:      http.StatusNotFound,
							contentType: "text/plain",
							body:        "not found",
							detail:      "not found",
						},
						{
							name:        "success",
							status:      http.StatusOK,
							contentType: "application/json",
							body:        `{"name":"pet"}`,
						},
					} {
						t.Run(response.name, func(t *testing.T) {
							statusCode, contentType, responseBody = response.status, response.contentType, response.body
							attempts.Store(0)

							result := any(map[string]any{"name": "pet"})
							if strings.HasSuffix(operation, "array") {
								result = []any{result}
								if statusCode == http.StatusOK {
									responseBody = "[" + responseBody + "]"
								}
							}

							endpoint := "/query"
							body := fmt.Sprintf(`{
								"collection":%q,
								"query":{"fields":{"__value":{"type":"column","column":"__value"}}},
								"arguments":{},"collection_relationships":{}
							}`, operation)
							if strings.HasPrefix(operation, "post") {
								endpoint = "/mutation"
								body = fmt.Sprintf(`{
									"operations":[{"type":"procedure","name":%q,"arguments":{}}],
									"collection_relationships":{}
								}`, operation)
							}

							res, err := http.Post(testServer.URL+endpoint, "application/json", bytes.NewBufferString(body))
							assert.NilError(t, err)

							wantAttempts := int32(1)
							if statusCode == http.StatusInternalServerError {
								wantAttempts += retry.times
							}
							assert.Equal(t, attempts.Load(), wantAttempts)

							if statusCode >= http.StatusBadRequest {
								wantStatus := statusCode
								if wantStatus < http.StatusInternalServerError {
									wantStatus = http.StatusUnprocessableEntity
								}
								assertHTTPResponse(t, res, wantStatus, schema.ErrorResponse{
									Message: fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
									Details: map[string]any{"error": response.detail},
								})
							} else if endpoint == "/query" {
								assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
									{Rows: []map[string]any{{"__value": result}}},
								})
							} else {
								assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
									OperationResults: []schema.MutationOperationResults{
										schema.NewProcedureResult(result).Encode(),
									},
								})
							}
						})
					}
				})
			}
		})
	}
}
