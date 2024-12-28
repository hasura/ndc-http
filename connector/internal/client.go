package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hasura/ndc-http/connector/internal/contenttype"
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

// HTTPClient represents a http client wrapper with advanced methods
type HTTPClient struct {
	manager  *UpstreamManager
	requests *RequestBuilderResults
}

// Send creates and executes the request and evaluate response selection
func (client *HTTPClient) Send(ctx context.Context, selection schema.NestedField) (any, http.Header, error) {
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

// execute a request to a list of remote servers in sequence
func (client *HTTPClient) sendSequence(ctx context.Context, requests []*RetryableRequest) (*DistributedResponse[any], http.Header) {
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

// execute a request to a list of remote servers in parallel
func (client *HTTPClient) sendParallel(ctx context.Context, requests []*RetryableRequest) (*DistributedResponse[any], http.Header) {
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

// execute a request to the remote server with retries
func (client *HTTPClient) sendSingle(ctx context.Context, request *RetryableRequest, mode string) (any, http.Header, *schema.ConnectorError) {
	ctx, span := tracer.Start(ctx, "Send Request to Server "+request.ServerID)
	defer span.End()

	span.SetAttributes(attribute.String("execution.mode", mode))

	requestURL := request.URL.String()
	rawPort := request.URL.Port()
	port := 80
	if rawPort != "" {
		if p, err := strconv.ParseInt(rawPort, 10, 32); err == nil {
			port = int(p)
		}
	} else if strings.HasPrefix(request.URL.Scheme, "https") {
		port = 443
	}

	logger := connector.GetLogger(ctx)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.String("request_url", requestURL),
			slog.String("request_method", request.RawRequest.Method),
			slog.Any("request_headers", request.Headers),
		}

		if request.Body != nil {
			logAttrs = append(logAttrs, slog.String("request_body", string(request.Body)))
		}
		logger.Debug("sending request to remote server...", logAttrs...)
	}

	contentEncoding := request.Headers.Get(rest.ContentEncodingHeader)
	if len(request.Body) > 0 && client.manager.compressors.IsEncodingSupported(contentEncoding) {
		var buf bytes.Buffer
		_, err := client.manager.compressors.Compress(&buf, contentEncoding, request.Body)
		if err != nil {
			span.SetStatus(codes.Error, "failed to execute the request")
			span.RecordError(err)

			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		request.Body = buf.Bytes()
	}

	var resp *http.Response
	var errorBytes []byte
	var err error
	var cancel context.CancelFunc

	times := int(request.Runtime.Retry.Times)
	delayMs := int(math.Max(float64(request.Runtime.Retry.Delay), 100))
	for i := 0; i <= times; i++ {
		resp, errorBytes, cancel, err = client.doRequest(ctx, request, port, i) //nolint:all
		if err != nil {
			span.SetStatus(codes.Error, "failed to execute the request")
			span.RecordError(err)

			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		if (resp.StatusCode >= 200 && resp.StatusCode < 299) ||
			!slices.Contains(request.Runtime.Retry.HTTPStatus, resp.StatusCode) || i >= times {
			break
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			logger.Debug(
				fmt.Sprintf("received error from remote server, retry %d of %d...", i+1, times),
				slog.Int("http_status", resp.StatusCode),
				slog.Any("response_headers", resp.Header),
				slog.String("response_body", string(errorBytes)),
			)
		}

		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	defer cancel()

	contentType := parseContentType(resp.Header.Get(rest.ContentTypeHeader))
	if resp.StatusCode >= 400 {
		details := make(map[string]any)
		switch contentType {
		case rest.ContentTypeJSON:
			if json.Valid(errorBytes) {
				details["error"] = json.RawMessage(errorBytes)
			} else {
				details["error"] = string(errorBytes)
			}
		case rest.ContentTypeXML:
			errData, err := contenttype.DecodeArbitraryXML(bytes.NewReader(errorBytes))
			if err != nil {
				details["error"] = string(errorBytes)
			} else {
				details["error"] = errData
			}
		default:
			details["error"] = string(errorBytes)
		}

		span.SetStatus(codes.Error, "received error from remote server")

		statusCode := resp.StatusCode
		if statusCode < 500 {
			statusCode = http.StatusUnprocessableEntity
		}

		return nil, nil, schema.NewConnectorError(statusCode, resp.Status, details)
	}

	result, headers, evalErr := client.evalHTTPResponse(ctx, span, resp, contentType, logger)
	if evalErr != nil {
		span.SetStatus(codes.Error, "failed to decode the http response")
		span.RecordError(evalErr)

		return nil, nil, evalErr
	}

	return result, headers, nil
}

func (client *HTTPClient) doRequest(ctx context.Context, request *RetryableRequest, port int, retryCount int) (*http.Response, []byte, context.CancelFunc, error) {
	method := strings.ToUpper(request.RawRequest.Method)
	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", method, request.RawRequest.URL), trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	urlAttr := cloneURL(&request.URL)
	password, hasPassword := urlAttr.User.Password()
	if urlAttr.User.String() != "" || hasPassword {
		maskedUser := restUtils.MaskString(urlAttr.User.Username())
		if hasPassword {
			urlAttr.User = url.UserPassword(maskedUser, restUtils.MaskString(password))
		} else {
			urlAttr.User = url.User(maskedUser)
		}
	}

	span.SetAttributes(
		attribute.String("db.system", "http"),
		attribute.String("http.request.method", method),
		attribute.String("url.full", urlAttr.String()),
		attribute.String("server.address", request.URL.Hostname()),
		attribute.Int("server.port", port),
		attribute.String("network.protocol.name", "http"),
	)

	var namespace string
	if client.requests.Schema != nil && client.requests.Schema.Name != "" {
		namespace = client.requests.Schema.Name
		span.SetAttributes(attribute.String("db.namespace", namespace))
	}

	if len(request.Body) > 0 {
		span.SetAttributes(attribute.Int("http.request.body.size", len(request.Body)))
	}
	if retryCount > 0 {
		span.SetAttributes(attribute.Int("http.request.resend_count", retryCount))
	}

	resp, cancel, err := client.manager.ExecuteRequest(ctx, span, request, namespace)
	if err != nil {
		span.SetStatus(codes.Error, "error happened when executing the request")
		span.RecordError(err)

		return nil, nil, nil, err
	}

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	setHeaderAttributes(span, "http.response.header.", resp.Header)

	if resp.ContentLength >= 0 {
		span.SetAttributes(attribute.Int64("http.response.size", resp.ContentLength))
	}

	resp.Body, err = client.manager.compressors.Decompress(resp.Body, resp.Header.Get(rest.ContentEncodingHeader))
	if err != nil {
		span.SetStatus(codes.Error, "error happened when decompressing the response body")
		span.RecordError(err)

		return nil, nil, nil, err
	}

	if resp.StatusCode < 300 {
		return resp, nil, cancel, nil
	}

	defer resp.Body.Close()
	span.SetStatus(codes.Error, "Non-2xx status")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
	} else {
		span.RecordError(errors.New(string(body)))
		span.SetAttributes(attribute.Int64("http.response.size", int64(len(body))))
	}

	return resp, body, cancel, nil
}

func (client *HTTPClient) evalHTTPResponse(ctx context.Context, span trace.Span, resp *http.Response, contentType string, logger *slog.Logger) (any, http.Header, *schema.ConnectorError) {
	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.Int("http_status", resp.StatusCode),
			slog.Any("response_headers", resp.Header),
		}
		if resp.Body != nil && resp.StatusCode != http.StatusNoContent {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if readErr != nil {
				span.SetStatus(codes.Error, "error happened when reading response body")
				span.RecordError(readErr)

				return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "error happened when reading response body", map[string]any{
					"error": readErr.Error(),
				})
			}
			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
			logAttrs = append(logAttrs, slog.String("response_body", string(respBody)))
		}

		logger.Debug("received response from remote server", logAttrs...)
	}

	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode == http.StatusNoContent {
		return true, resp.Header, nil
	}

	if resp.Body == nil || resp.ContentLength == 0 {
		return nil, resp.Header, nil
	}

	resultType := client.requests.Operation.OriginalResultType

	switch {
	case restUtils.IsContentTypeText(contentType):
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return string(respBody), resp.Header, nil
	case restUtils.IsContentTypeXML(contentType):
		var err error
		result, err := contenttype.NewXMLDecoder(client.requests.Schema.NDCHttpSchema).Decode(resp.Body, resultType)
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return result, resp.Header, nil
	case restUtils.IsContentTypeJSON(contentType):
		if len(resultType) > 0 {
			namedType, err := resultType.AsNamed()
			if err == nil && namedType.Name == string(rest.ScalarString) {
				respBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "failed to read response", map[string]any{
						"reason": err.Error(),
					})
				}

				var strResult string
				if err := json.Unmarshal(respBytes, &strResult); err != nil {
					// fallback to raw string response if the result type is String
					return string(respBytes), resp.Header, nil
				}

				return strResult, resp.Header, nil
			}
		}

		var result any
		var err error
		if client.requests.Schema == nil || client.requests.Schema.NDCHttpSchema == nil {
			err = json.NewDecoder(resp.Body).Decode(&result)
		} else {
			result, err = contenttype.NewJSONDecoder(client.requests.Schema.NDCHttpSchema).Decode(resp.Body, resultType)
		}

		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return result, resp.Header, nil
	case contentType == rest.ContentTypeNdJSON:
		var results []any
		decoder := json.NewDecoder(resp.Body)
		for decoder.More() {
			var r any
			err := decoder.Decode(&r)
			if err != nil {
				return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
			}
			results = append(results, r)
		}

		return results, resp.Header, nil
	case restUtils.IsContentTypeBinary(contentType):
		rawBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		return base64.StdEncoding.EncodeToString(rawBytes), resp.Header, nil
	default:
		return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "failed to evaluate response", map[string]any{
			"cause": "unsupported content type " + contentType,
		})
	}
}

func (client *HTTPClient) createHeaderForwardingResponse(result any, rawHeaders http.Header) any {
	forwardHeaders := client.manager.config.ForwardHeaders
	if !forwardHeaders.Enabled || forwardHeaders.ResponseHeaders == nil {
		return result
	}

	headers := make(map[string]string)
	for key, values := range rawHeaders {
		if len(forwardHeaders.ResponseHeaders.ForwardHeaders) > 0 && !slices.Contains(forwardHeaders.ResponseHeaders.ForwardHeaders, key) {
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
	if input == "" {
		return ""
	}
	parts := strings.Split(input, ";")
	contentTypeParts := strings.Split(parts[0], ",")

	return strings.TrimSpace(contentTypeParts[0])
}
