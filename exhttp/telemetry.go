package exhttp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/hasura/ndc-sdk-go/connector"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TelemetryConfig hold optional options for the telemetry round tripper.
type TelemetryConfig struct {
	Tracer                     trace.Tracer
	Logger                     *slog.Logger
	Propagator                 propagation.TextMapPropagator
	Port                       int
	Attributes                 []attribute.KeyValue
	DisableHighCardinalityPath bool
}

// Validate applies default values.
func (tc *TelemetryConfig) Validate() {
	if tc.Tracer == nil {
		tc.Tracer = otel.Tracer("github.com/hasura/ndc-http/exhttp")
	}

	if tc.Propagator == nil {
		tc.Propagator = otel.GetTextMapPropagator()
	}
}

func (tt TelemetryConfig) do(fn func(req *http.Request) (*http.Response, error), req *http.Request) (*http.Response, error) {
	ctx, span := tt.Tracer.Start(req.Context(), tt.getRequestSpanName(req), trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	port := tt.Port
	if port == 0 {
		port, _ = ParsePort(req.URL.Port(), req.URL.Scheme)
	}

	urlAttr := *req.URL
	password, hasPassword := urlAttr.User.Password()
	if urlAttr.User.String() != "" || hasPassword {
		maskedUser := strings.Repeat("x", len(urlAttr.User.Username()))
		if hasPassword {
			urlAttr.User = url.UserPassword(maskedUser, strings.Repeat("x", len(password)))
		} else {
			urlAttr.User = url.User(maskedUser)
		}
	}

	if len(tt.Attributes) > 0 {
		span.SetAttributes(tt.Attributes...)
	}

	span.SetAttributes(
		attribute.String("http.request.method", req.Method),
		attribute.String("url.full", req.URL.String()),
		attribute.String("server.address", req.URL.Hostname()),
		attribute.Int("server.port", port),
		attribute.String("network.protocol.name", "http"),
	)

	connector.SetSpanHeaderAttributes(span, "http.request.header.", req.Header)

	if req.ContentLength > 0 {
		span.SetAttributes(attribute.Int64("http.request.body.size", req.ContentLength))
	}

	isDebug := tt.isDebug(ctx)
	tt.Propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))
	requestLogAttrs := map[string]any{
		"url":     req.URL.String(),
		"method":  req.Method,
		"headers": connector.NewTelemetryHeaders(req.Header),
	}

	if isDebug && req.Body != nil && req.ContentLength > 0 && req.ContentLength <= 100*1024 {
		rawBody, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		requestLogAttrs["body"] = string(rawBody)

		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(rawBody))
	}

	logAttrs := []slog.Attr{
		slog.String("type", "http-client"),
		slog.Any("request", requestLogAttrs),
	}

	resp, err := fn(req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		tt.printLog(ctx, slog.LevelDebug, "failed to execute the request: "+err.Error(), logAttrs...)

		return resp, err
	}

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	connector.SetSpanHeaderAttributes(span, "http.response.header.", resp.Header)

	if resp.ContentLength >= 0 {
		span.SetAttributes(attribute.Int64("http.response.size", resp.ContentLength))
	}

	respLogAttrs := map[string]any{
		"http_status": resp.StatusCode,
		"headers":     resp.Header,
	}

	if isDebug && resp.ContentLength > 0 && resp.ContentLength < 1024*1024 && resp.Body != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			logAttrs = append(logAttrs, slog.Any("response", respLogAttrs))

			tt.printLog(ctx, slog.LevelDebug, "failed to read response body: "+err.Error(), logAttrs...)
			resp.Body.Close()

			return resp, err
		}

		respLogAttrs["body"] = string(respBody)
		logAttrs = append(logAttrs, slog.Any("response", respLogAttrs))

		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewBuffer(respBody))

		span.SetAttributes(attribute.Int("http.response.size", len(respBody)))
	}

	tt.printLog(ctx, slog.LevelDebug, resp.Status, logAttrs...)

	if resp.StatusCode >= http.StatusBadRequest {
		span.SetStatus(codes.Error, resp.Status)
	}

	return resp, err
}

func (tt TelemetryConfig) getRequestSpanName(req *http.Request) string {
	spanName := req.Method
	if !tt.DisableHighCardinalityPath {
		spanName += " " + req.URL.Path
	}

	return spanName
}

func (tt TelemetryConfig) isDebug(ctx context.Context) bool {
	return tt.Logger != nil && tt.Logger.Enabled(ctx, slog.LevelDebug)
}

func (tt TelemetryConfig) printLog(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if tt.Logger == nil {
		return
	}

	tt.Logger.LogAttrs(ctx, level, msg, attrs...)
}

type telemetryTransport struct {
	transport http.RoundTripper
	TelemetryConfig
}

// NewTelemetryTransport creates a new transport with telemetry.
func NewTelemetryTransport(transport http.RoundTripper, config TelemetryConfig) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	config.Validate()

	return telemetryTransport{
		transport:       transport,
		TelemetryConfig: config,
	}
}

// RoundTrip wraps the base RoundTripper with telemetry.
func (tt telemetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return tt.TelemetryConfig.do(tt.transport.RoundTrip, req)
}

// TelemetryMiddleware wraps the client with logging and tracing.
type TelemetryMiddleware struct {
	doer Doer
	TelemetryConfig
}

// NewTelemetryMiddleware creates a new transport with telemetry.
func NewTelemetryMiddleware(config TelemetryConfig) Middleware {
	return func(doer Doer) Doer {
		config.Validate()

		return &TelemetryMiddleware{
			doer:            doer,
			TelemetryConfig: config,
		}
	}
}

// Do wraps the base Doer with telemetry.
func (tm TelemetryMiddleware) Do(req *http.Request) (*http.Response, error) {
	return tm.TelemetryConfig.do(tm.doer.Do, req)
}
