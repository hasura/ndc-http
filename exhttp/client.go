package exhttp

import (
	"io"
	"net/http"

	"github.com/hasura/ndc-sdk-go/utils/compression"
)

const contentEncodingHeader = "Content-Encoding"

// Doer abstracts an HTTP client interface.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Middleware abstracts a function to create Doer.
type Middleware func(doer Doer) Doer

// Client is an http client wrapper with retry and implement the http client interface.
type Client struct {
	doer Doer
}

// NewClient creates a new client instance.
func NewClient(doer Doer, middlewares ...Middleware) *Client {
	if doer == nil {
		doer = http.DefaultClient
	}

	for _, apply := range middlewares {
		doer = apply(doer)
	}

	return &Client{doer: doer}
}

// Do sends an HTTP request and returns an HTTP response,
// following policy (such as redirects, cookies, auth) as configured on the client.
func (r *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept-Encoding", compression.DefaultCompressor.AcceptEncoding())

	client := r.doer
	if r.doer == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		respBody, dcErr := compression.DefaultCompressor.Decompress(
			resp.Body,
			resp.Header.Get(contentEncodingHeader),
		)
		if dcErr != nil && err == nil {
			return resp, dcErr
		}

		resp.Body = respBody
	}

	return resp, err
}

// Get issues a GET to the specified URL.
func (r *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return nil, err
	}

	return r.Do(req)
}

// Post issues a POST to the specified URL.
func (r *Client) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body) //nolint:noctx
	if err != nil {
		return nil, err
	}

	if bodyType != "" {
		req.Header.Set("Content-Type", bodyType)
	}

	return r.Do(req)
}
