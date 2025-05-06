package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/hasura/ndc-http/connector/internal/contenttype"
	"github.com/hasura/ndc-http/exhttp"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

var tracer = connector.NewTracer("HTTPClient")

// HTTPClient represents a http client wrapper with advanced methods.
type HTTPClient struct {
	manager  *UpstreamManager
	requests *RequestBuilderResults
}

// Send creates and executes the request and evaluate response selection.
func (client *HTTPClient) Send(
	ctx context.Context,
	selection schema.NestedField,
) (any, http.Header, error) {
	httpOptions := client.requests.HTTPOptions

	var result any

	var headers http.Header

	switch {
	case !httpOptions.Distributed:
		var err *schema.ConnectorError

		result, headers, err = client.sendSingle(ctx, client.requests.Requests[0], "single")
		if err != nil {
			return nil, nil, err
		}
	case !httpOptions.Parallel || httpOptions.Concurrency <= 1 || len(client.requests.Requests) == 1:
		rs, hs := client.sendSequence(ctx, client.requests.Requests)
		headers = hs
		result = rs.ToMap()
	default:
		rs, hs := client.sendParallel(ctx, client.requests.Requests)
		headers = hs
		result = rs.ToMap()
	}

	result = client.createHeaderForwardingResponse(result, headers)

	if len(selection) > 0 {
		var err error

		result, err = utils.EvalNestedColumnFields(selection, result)
		if err != nil {
			return nil, nil, schema.InternalServerError(err.Error(), nil)
		}
	}

	return result, headers, nil
}

// execute a request to a list of remote servers in sequence.
func (client *HTTPClient) sendSequence(
	ctx context.Context,
	requests []*RetryableRequest,
) (*DistributedResponse[any], http.Header) {
	results := NewDistributedResponse[any]()

	var firstHeaders http.Header

	for _, req := range requests {
		result, headers, err := client.sendSingle(ctx, req, "sequence")
		if err != nil {
			results.Errors = append(results.Errors, DistributedError{
				Server:         req.ServerID,
				ConnectorError: *err,
			})
		} else {
			results.Results = append(results.Results, DistributedResult[any]{
				Server: req.ServerID,
				Data:   result,
			})

			if firstHeaders == nil {
				firstHeaders = headers
			}
		}
	}

	return results, firstHeaders
}

// execute a request to a list of remote servers in parallel.
func (client *HTTPClient) sendParallel(
	ctx context.Context,
	requests []*RetryableRequest,
) (*DistributedResponse[any], http.Header) {
	var firstHeaders http.Header

	httpOptions := client.requests.HTTPOptions
	results := make([]*DistributedResult[any], len(requests))
	errs := make([]*DistributedError, len(requests))

	eg, ctx := errgroup.WithContext(ctx)
	if httpOptions.Concurrency > 0 {
		eg.SetLimit(int(httpOptions.Concurrency))
	}

	sendFunc := func(req RetryableRequest, index int) {
		eg.Go(func() error {
			result, headers, err := client.sendSingle(ctx, &req, "parallel")
			if err != nil {
				errs[index] = &DistributedError{
					Server:         req.ServerID,
					ConnectorError: *err,
				}
			} else {
				results[index] = &DistributedResult[any]{
					Server: req.ServerID,
					Data:   result,
				}

				if firstHeaders == nil {
					firstHeaders = headers
				}
			}

			return nil
		})
	}

	for i, req := range requests {
		sendFunc(*req, i)
	}

	_ = eg.Wait()

	r := NewDistributedResponse[any]()

	for _, item := range results {
		if item != nil {
			r.Results = append(r.Results, *item)
		}
	}

	for _, err := range errs {
		if err != nil {
			r.Errors = append(r.Errors, *err)
		}
	}

	return r, firstHeaders
}

// execute a request to the remote server with retries.
func (client *HTTPClient) sendSingle(
	ctx context.Context,
	request *RetryableRequest,
	mode string,
) (any, http.Header, *schema.ConnectorError) {
	logger := connector.GetLogger(ctx)

	ctx, span := tracer.Start(ctx, "Send Request to Server "+request.ServerID)
	defer span.End()

	span.SetAttributes(attribute.String("execution.mode", mode))

	var namespace string

	var httpError *exhttp.HTTPError

	if client.requests.Schema != nil && client.requests.Schema.Name != "" {
		namespace = client.requests.Schema.Name
		span.SetAttributes(attribute.String("db.namespace", namespace))
	}

	resp, cancel, err := client.manager.ExecuteRequest(ctx, span, request, namespace)
	if err != nil {
		span.SetStatus(codes.Error, "error happened when executing the request")
		span.RecordError(err)

		if !errors.As(err, &httpError) {
			return nil, nil, schema.InternalServerError(err.Error(), nil)
		}
	}

	defer func() {
		cancel()

		_ = resp.Body.Close()
	}()

	contentType := parseContentType(resp.Header.Get(rest.ContentTypeHeader))

	if httpError != nil {
		details := make(map[string]any)

		switch contentType {
		case rest.ContentTypeJSON:
			if json.Valid(httpError.Body) {
				details["error"] = json.RawMessage(httpError.Body)
			} else {
				details["error"] = string(httpError.Body)
			}
		case rest.ContentTypeXML:
			errData, err := contenttype.DecodeArbitraryXML(bytes.NewReader(httpError.Body))
			if err != nil {
				details["error"] = string(httpError.Body)
			} else {
				details["error"] = errData
			}
		default:
			details["error"] = string(httpError.Body)
		}

		statusCode := resp.StatusCode
		if statusCode < http.StatusInternalServerError {
			statusCode = http.StatusUnprocessableEntity
		}

		return nil, nil, schema.NewConnectorError(statusCode, resp.Status, details)
	}

	result, evalErr := client.evalHTTPResponse(ctx, span, resp, contentType, logger)
	if evalErr != nil {
		// return the null result if the status code is no content.
		if resp.StatusCode == http.StatusNoContent {
			return nil, resp.Header, nil
		}

		span.SetStatus(codes.Error, "failed to decode the http response")
		span.RecordError(evalErr)

		return nil, nil, evalErr
	}

	transformedResult, err := client.transformResponse(result)
	if err != nil {
		span.SetStatus(codes.Error, "failed to transform the http response")
		span.RecordError(err)

		return nil, nil, schema.InternalServerError(err.Error(), nil)
	}

	return transformedResult, resp.Header, nil
}

func (client *HTTPClient) evalHTTPResponse(
	ctx context.Context,
	span trace.Span,
	resp *http.Response,
	contentType string,
	logger *slog.Logger,
) (any, *schema.ConnectorError) {
	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.Int("http_status", resp.StatusCode),
			slog.Any("response_headers", resp.Header),
		}

		if resp.Body != nil {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if readErr != nil {
				span.SetStatus(codes.Error, "error happened when reading response body")
				span.RecordError(readErr)

				return nil, schema.NewConnectorError(
					http.StatusInternalServerError,
					"error happened when reading response body",
					map[string]any{
						"error": readErr.Error(),
					},
				)
			}

			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
			logAttrs = append(logAttrs, slog.String("response_body", string(respBody)))
		}

		logger.Debug("received response from remote server", logAttrs...)
	}

	if resp.Body == nil || resp.ContentLength == 0 {
		return nil, nil
	}

	resultType := client.requests.Operation.OriginalResultType

	switch {
	case restUtils.IsContentTypeText(contentType):
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return string(respBody), nil
	case restUtils.IsContentTypeXML(contentType):
		var err error

		result, err := contenttype.NewXMLDecoder(client.requests.Schema.NDCHttpSchema).
			Decode(resp.Body, resultType)
		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return result, nil
	case restUtils.IsContentTypeJSON(contentType):
		if len(resultType) > 0 {
			namedType, err := resultType.AsNamed()
			if err == nil && namedType.Name == string(rest.ScalarString) {
				respBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, schema.NewConnectorError(
						http.StatusInternalServerError,
						"failed to read response",
						map[string]any{
							"reason": err.Error(),
						},
					)
				}

				var strResult string
				if err := json.Unmarshal(respBytes, &strResult); err != nil {
					// fallback to raw string response if the result type is String
					return string(respBytes), nil //nolint:nilerr
				}

				return strResult, nil
			}
		}

		var result any

		var err error

		if client.requests.Schema == nil || client.requests.Schema.NDCHttpSchema == nil {
			if client.manager.RuntimeSettings.StringifyJSON {
				// read, validate the json string and returns the raw value
				resultBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, schema.NewConnectorError(
						http.StatusInternalServerError,
						err.Error(),
						nil,
					)
				}

				if len(resultBytes) == 0 {
					return nil, nil
				}

				return string(resultBytes), nil
			}

			err = json.NewDecoder(resp.Body).Decode(&result)
		} else {
			result, err = contenttype.NewJSONDecoder(client.requests.Schema.NDCHttpSchema, contenttype.JSONDecodeOptions{
				StringifyJSON: client.manager.RuntimeSettings.StringifyJSON,
			}).Decode(resp.Body, resultType)
		}

		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return result, nil
	case contentType == rest.ContentTypeNdJSON:
		var results []any

		decoder := json.NewDecoder(resp.Body)
		for decoder.More() {
			var r any

			err := decoder.Decode(&r)
			if err != nil {
				return nil, schema.NewConnectorError(
					http.StatusInternalServerError,
					err.Error(),
					nil,
				)
			}

			results = append(results, r)
		}

		return results, nil
	case restUtils.IsContentTypeBinary(contentType):
		rawBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return base64.StdEncoding.EncodeToString(rawBytes), nil
	default:
		return nil, schema.NewConnectorError(
			http.StatusInternalServerError,
			"failed to evaluate response",
			map[string]any{
				"cause": "unsupported content type " + contentType,
			},
		)
	}
}

func (client *HTTPClient) createHeaderForwardingResponse(result any, rawHeaders http.Header) any {
	forwardHeaders := client.manager.config.ForwardHeaders
	if !forwardHeaders.Enabled || forwardHeaders.ResponseHeaders == nil {
		return result
	}

	headers := make(map[string]string)

	for key, values := range rawHeaders {
		if len(forwardHeaders.ResponseHeaders.ForwardHeaders) > 0 &&
			!slices.Contains(forwardHeaders.ResponseHeaders.ForwardHeaders, key) {
			continue
		}

		if len(values) > 0 && values[0] != "" {
			headers[key] = values[0]
		}
	}

	return map[string]any{
		forwardHeaders.ResponseHeaders.HeadersField: headers,
		forwardHeaders.ResponseHeaders.ResultField:  result,
	}
}

func parseContentType(input string) string {
	cts := strings.FieldsFunc(input, func(r rune) bool {
		return unicode.IsSpace(r) || r == ';' || r == ','
	})

	if len(cts) == 0 {
		return ""
	}

	return strings.ToLower(cts[0])
}
