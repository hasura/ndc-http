package exhttp

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/hasura/goenvconf"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"github.com/hasura/ndc-sdk-go/v2/utils"
)

var defaultRetryHTTPStatus = []int{408, 429, 500, 502, 503}

// RetryPolicySetting represents retry policy settings.
type RetryPolicySetting struct {
	// Number of retry times
	Times *goenvconf.EnvInt `json:"times,omitempty" mapstructure:"times" yaml:"times,omitempty"`
	// The initial wait time in milliseconds before a retry is attempted.
	// Must be >0. Defaults to 1 second.
	Delay *goenvconf.EnvInt `json:"delay,omitempty" mapstructure:"delay" yaml:"delay,omitempty"`
	// HTTPStatus retries if the remote service returns one of these http status
	HTTPStatus []int `json:"httpStatus,omitempty" mapstructure:"httpStatus" yaml:"httpStatus,omitempty"`

	// How much does the reconnection time vary relative to the base value.
	// This is useful to prevent multiple clients to reconnect at the exact
	// same time, as it makes the wait times distinct.
	// Must be in range (0, 1); Defaults to 0.5.
	Jitter *float64 `json:"jitter,omitempty" jsonschema:"nullable,min=0,max=1" mapstructure:"jitter" yaml:"jitter,omitempty"`
	// How much should the reconnection time grow on subsequent attempts.
	// Must be >=1; 1 = constant interval. Defaults to 1.5.
	Multiplier float64 `json:"multiplier,omitempty" jsonschema:"min=1" mapstructure:"multiplier" yaml:"multiplier,omitempty"`
	// How much can the wait time in seconds grow. Defaults to 60 seconds.
	MaxIntervalSeconds uint `json:"maxIntervalSeconds,omitempty" jsonschema:"nullable,min=0" mapstructure:"maxIntervalSeconds" yaml:"maxIntervalSeconds,omitempty"`
	// Maximum total time in seconds for all retries.
	MaxElapsedTimeSeconds uint `json:"maxElapsedTimeSeconds,omitempty" jsonschema:"nullable,min=0" mapstructure:"maxElapsedTimeSeconds" yaml:"maxElapsedTimeSeconds,omitempty"`
}

// Validate if the current instance is valid.
func (rs RetryPolicySetting) Validate() (*RetryPolicy, error) {
	var (
		errs         []error
		err          error
		times, delay int64
	)

	if rs.Times != nil {
		times, err = rs.Times.Get()
		if err != nil {
			errs = append(errs, err)
		} else if times < 0 {
			errs = append(errs, errors.New("retry policy times must be positive"))
		}
	}

	if rs.Delay != nil {
		delay, err = rs.Delay.Get()
		if err != nil {
			errs = append(errs, err)
		} else if delay < 0 {
			errs = append(errs, errors.New("retry delay must be larger than 0"))
		}
	}

	for _, status := range rs.HTTPStatus {
		if status < 400 || status >= 600 {
			errs = append(errs, errors.New("retry http status must be in between 400 and 599"))

			break
		}
	}

	result := &RetryPolicy{
		Times:                 uint(times),
		Delay:                 uint(delay),
		HTTPStatus:            rs.HTTPStatus,
		Multiplier:            backoff.DefaultMultiplier,
		MaxElapsedTimeSeconds: uint(backoff.DefaultMaxElapsedTime / time.Second),
	}

	if rs.Jitter != nil {
		if *rs.Jitter < 0 || *rs.Jitter > 1 {
			errs = append(errs, errors.New("jitter must be in range (0, 1)"))
		} else {
			result.Jitter = rs.Jitter
		}
	}

	if rs.Multiplier != 0 {
		if rs.Multiplier < 1 {
			errs = append(errs, errors.New("retry multiplier must be >= 1"))
		} else {
			result.Multiplier = rs.Multiplier
		}
	}

	if rs.MaxIntervalSeconds != 0 {
		result.MaxIntervalSeconds = rs.MaxIntervalSeconds
	}

	if rs.MaxElapsedTimeSeconds != 0 {
		result.MaxElapsedTimeSeconds = rs.MaxElapsedTimeSeconds
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}

// RetryPolicy represents the retry policy of request.
type RetryPolicy struct {
	// Number of retry times. Defaults to 0 (no retry).
	Times uint `json:"times,omitempty" mapstructure:"times" yaml:"times,omitempty"`
	// Delay retry delay in milliseconds. Defaults to 1 second
	Delay uint `json:"delay,omitempty" mapstructure:"delay" yaml:"delay,omitempty"`
	// HTTPStatus retries if the remote service returns one of these http status
	HTTPStatus []int `json:"httpStatus,omitempty" mapstructure:"httpStatus" yaml:"httpStatus,omitempty"`
	// How much does the reconnection time vary relative to the base value.
	// This is useful to prevent multiple clients to reconnect at the exact
	// same time, as it makes the wait times distinct.
	// Must be in range (0, 1); Defaults to 0.5.
	Jitter *float64 `json:"jitter,omitempty" mapstructure:"jitter" yaml:"jitter,omitempty"`
	// How much should the reconnection time grow on subsequent attempts.
	// Must be >=1; 1 = constant interval. Defaults to 1.5.
	Multiplier float64 `json:"multiplier,omitempty" mapstructure:"multiplier" yaml:"multiplier,omitempty"`
	// How much can the wait time grow.
	// If <=0 = the wait time can infinitely grow. Defaults to 60 seconds.
	MaxIntervalSeconds uint `json:"maxIntervalSeconds,omitempty" mapstructure:"maxIntervalSeconds" yaml:"maxIntervalSeconds,omitempty"`
	// Maximum total time in seconds for all retries.
	MaxElapsedTimeSeconds uint `json:"maxElapsedTimeSeconds,omitempty" mapstructure:"maxElapsedTimeSeconds" yaml:"maxElapsedTimeSeconds,omitempty"`
}

// GetMaxElapsedTime returns the max elapsed time duration.
func (rp RetryPolicy) GetMaxElapsedTime() time.Duration {
	if rp.MaxElapsedTimeSeconds > 0 {
		return time.Duration(rp.MaxElapsedTimeSeconds) * time.Second
	}

	return backoff.DefaultMaxElapsedTime
}

// GetRetryHTTPStatus returns the http status to be retried.
func (rp RetryPolicy) GetRetryHTTPStatus() []int {
	if len(rp.HTTPStatus) == 0 {
		return defaultRetryHTTPStatus
	}

	return rp.HTTPStatus
}

// GetExponentialBackoff returns a new GetExponentialBackoff config.
func (rp RetryPolicy) GetExponentialBackoff() *backoff.ExponentialBackOff {
	result := backoff.NewExponentialBackOff()

	if rp.Delay > 0 {
		result.InitialInterval = time.Duration(rp.Delay) * time.Millisecond
	}

	if rp.Jitter != nil {
		result.RandomizationFactor = *rp.Jitter
	}

	if rp.Multiplier >= 1 {
		result.Multiplier = rp.Multiplier
	}

	if rp.MaxIntervalSeconds > 0 {
		result.MaxInterval = time.Duration(rp.MaxIntervalSeconds) * time.Second
	}

	return result
}

// Schema returns the object type schema of this type.
func (rp RetryPolicy) Schema() schema.ObjectType {
	return schema.ObjectType{
		Description: utils.ToPtr("Retry policy of request"),
		Fields: schema.ObjectTypeFields{
			"times": {
				Description: utils.ToPtr("Number of retry times"),
				Type:        schema.NewNamedType("Int32").Encode(),
			},
			"delay": {
				Description: utils.ToPtr(
					"The initial wait time in milliseconds before a retry is attempted.",
				),
				Type: schema.NewNullableType(schema.NewNamedType("Int32")).Encode(),
			},
			"httpStatus": {
				Description: utils.ToPtr("List of HTTP status the connector will retry on"),
				Type: schema.NewNullableType(schema.NewArrayType(schema.NewNamedType("Int32"))).
					Encode(),
			},
			"jitter": {
				Description: utils.ToPtr(
					"How much does the reconnection time vary relative to the base value. Must be in range (0, 1)",
				),
				Type: schema.NewNullableType(schema.NewNamedType("Float64")).Encode(),
			},
			"multiplier": {
				Description: utils.ToPtr(
					"How much should the reconnection time grow on subsequent attempts. Must be >=1; 1 = constant interval",
				),
				Type: schema.NewNullableType(schema.NewNamedType("Float64")).Encode(),
			},
			"maxIntervalSeconds": {
				Description: utils.ToPtr("How much can the wait time grow. Defaults to 60 seconds"),
				Type:        schema.NewNullableType(schema.NewNamedType("Float64")).Encode(),
			},
		},
	}
}

type retryMiddleware struct {
	doer   Doer
	config RetryPolicy
}

func NewRetryMiddleware(config RetryPolicy) Middleware {
	return func(doer Doer) Doer {
		return &retryMiddleware{
			doer:   doer,
			config: config,
		}
	}
}

// Do sends an HTTP request and returns an HTTP response,
// following policy (such as redirects, cookies, auth) as configured on the client.
func (r *retryMiddleware) Do(req *http.Request) (*http.Response, error) {
	if r.config.Times == 0 {
		return r.doer.Do(req)
	}

	var reqBody io.ReadSeeker

	if req.Body != nil {
		if bodySeeker, ok := req.Body.(io.ReadSeeker); !ok {
			rawBytes, err := io.ReadAll(req.Body)
			_ = req.Body.Close()

			if err != nil {
				return nil, err
			}

			reqBody = bytes.NewReader(rawBytes)
		} else {
			reqBody = bodySeeker
		}
	}

	var httpErr error

	operation := func() (*http.Response, error) {
		if reqBody != nil {
			_, _ = reqBody.Seek(0, io.SeekStart)
			req.Body = io.NopCloser(reqBody)
		}

		resp, err := r.doer.Do(req)
		if err != nil {
			return nil, backoff.Permanent(err)
		}

		// In case on non-retriable error, return Permanent error to stop retrying.
		if resp.StatusCode < 400 {
			return resp, nil
		}

		httpErr = HTTPErrorFromResponse(resp)

		if !slices.Contains(r.config.GetRetryHTTPStatus(), resp.StatusCode) {
			return resp, backoff.Permanent(httpErr)
		}

		retryAfter := getRetryAfter(resp)
		if retryAfter > 0 {
			return resp, backoff.RetryAfter(retryAfter)
		}

		return resp, httpErr
	}

	resp, err := backoff.Retry(
		req.Context(),
		operation,
		backoff.WithBackOff(r.config.GetExponentialBackoff()),
		backoff.WithMaxElapsedTime(r.config.GetMaxElapsedTime()),
		backoff.WithMaxTries(r.config.Times+1),
	)
	if err == nil {
		return resp, nil
	}

	var permanentErr *backoff.PermanentError
	if errors.As(err, &permanentErr) {
		return resp, permanentErr.Err
	}

	if httpErr != nil {
		return resp, httpErr
	}

	return resp, err
}

// The HTTP [Retry-After] response header indicates how long the user agent should wait before making a follow-up request.
// The client finds this header if exist and decodes to duration.
// If the header doesn't exist or there is any error happened, fallback to the retry delay setting.
//
// [Retry-After]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After
func getRetryAfter(resp *http.Response) int {
	rawRetryAfter := resp.Header.Get("Retry-After")
	if rawRetryAfter == "" {
		return 0
	}

	// A non-negative decimal integer indicating the seconds to delay after the response is received.
	retryAfterSecs, err := strconv.Atoi(rawRetryAfter)
	if err == nil && retryAfterSecs > 0 {
		return retryAfterSecs
	}

	// A date after which to retry, e.g. Tue, 29 Oct 2024 16:56:32 GMT
	retryTime, err := time.Parse(time.RFC1123, rawRetryAfter)
	if err == nil && retryTime.After(time.Now()) {
		duration := time.Until(retryTime)

		return int(duration.Seconds())
	}

	return 0
}
