package connector

import (
	"errors"
	"net/http"

	"github.com/hasura/gotel"
)

const (
	// valueFieldKey is the NDC field key holding a function's scalar result.
	valueFieldKey = "__value"
	// errorKeyCause is the key used in structured error detail maps.
	errorKeyCause = "cause"
)

var errBuildSchemaFailed = errors.New("failed to build NDC HTTP schema")

// State is the global state which is shared for every connector request.
type State struct {
	Tracer *gotel.Tracer
}

type options struct {
	client *http.Client
}

var defaultOptions options = options{
	client: &http.Client{
		Transport: http.DefaultTransport,
	},
}

// Option is an interface to set custom HTTP connector options.
type Option (func(*options))

// WithClient sets the custom HTTP client that satisfy the Doer interface.
func WithClient(client *http.Client) Option {
	return func(opts *options) {
		opts.client = client
	}
}
