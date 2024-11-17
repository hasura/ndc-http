package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// RetryableRequest wraps the raw request with retryable
type RetryableRequest struct {
	RawRequest    *rest.Request
	URL           url.URL
	ServerID      string
	ContentType   string
	ContentLength int64
	Headers       http.Header
	Body          io.ReadSeeker
	Runtime       rest.RuntimeSettings
}

// CreateRequest creates an HTTP request with body copied
func (r *RetryableRequest) CreateRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	if r.Body != nil {
		_, err := r.Body.Seek(0, io.SeekStart)
		if err != nil {
			return nil, nil, err
		}
	}

	timeout := r.Runtime.Timeout
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	}

	ctxR, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(r.RawRequest.Method), r.URL.String(), r.Body)
	if err != nil {
		cancel()

		return nil, nil, err
	}
	for key, header := range r.Headers {
		request.Header[key] = header
	}
	request.Header.Set(rest.ContentTypeHeader, r.ContentType)

	return request, cancel, nil
}

func getBaseURLFromServers(servers []rest.ServerConfig, serverIDs []string) (*url.URL, string) {
	var results []url.URL
	var selectedServerIDs []string
	for _, server := range servers {
		if len(serverIDs) > 0 && !slices.Contains(serverIDs, server.ID) {
			continue
		}
		hostPtr, err := server.GetURL()
		if err == nil {
			results = append(results, hostPtr)
			selectedServerIDs = append(selectedServerIDs, server.ID)
		}
	}

	switch len(results) {
	case 0:
		return nil, ""
	case 1:
		result := results[0]

		return &result, selectedServerIDs[0]
	default:
		index := rand.IntN(len(results) - 1)
		host := results[index]

		return &host, selectedServerIDs[index]
	}
}

// BuildDistributedRequestsWithOptions builds distributed requests with options
func BuildDistributedRequestsWithOptions(request *RetryableRequest, httpOptions *HTTPOptions) ([]RetryableRequest, error) {
	if strings.HasPrefix(request.URL.Scheme, "http") {
		return []RetryableRequest{*request}, nil
	}

	if !httpOptions.Distributed || len(httpOptions.Settings.Servers) == 1 {
		baseURL, serverID := getBaseURLFromServers(httpOptions.Settings.Servers, httpOptions.Servers)
		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.URL.Path = baseURL.Path + request.URL.Path
		request.ServerID = serverID
		if err := request.applySettings(httpOptions.Settings, httpOptions.Explain); err != nil {
			return nil, err
		}

		return []RetryableRequest{*request}, nil
	}

	var requests []RetryableRequest
	var buf []byte
	var err error
	if httpOptions.Parallel && request.Body != nil {
		// copy new readers for each requests to avoid race condition
		buf, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}
	serverIDs := httpOptions.Servers
	if len(serverIDs) == 0 {
		for _, server := range httpOptions.Settings.Servers {
			serverIDs = append(serverIDs, server.ID)
		}
	}
	for _, serverID := range serverIDs {
		baseURL, serverID := getBaseURLFromServers(httpOptions.Settings.Servers, []string{serverID})
		if baseURL == nil {
			continue
		}
		baseURL.Path += request.URL.Path
		baseURL.RawQuery = request.URL.RawQuery
		baseURL.Fragment = request.URL.Fragment
		req := RetryableRequest{
			URL:         *baseURL,
			ServerID:    serverID,
			RawRequest:  request.RawRequest,
			ContentType: request.ContentType,
			Headers:     request.Headers.Clone(),
			Body:        request.Body,
		}
		if err := req.applySettings(httpOptions.Settings, httpOptions.Explain); err != nil {
			return nil, err
		}
		if len(buf) > 0 {
			req.Body = bytes.NewReader(buf)
		}
		requests = append(requests, req)
	}

	return requests, nil
}

func (req *RetryableRequest) getServerConfig(settings *rest.NDCHttpSettings) *rest.ServerConfig {
	if settings == nil {
		return nil
	}
	if req.ServerID == "" {
		return &settings.Servers[0]
	}
	for _, server := range settings.Servers {
		if server.ID == req.ServerID {
			return &server
		}
	}

	return nil
}

func (req *RetryableRequest) applySecurity(serverConfig *rest.ServerConfig, isExplain bool) error {
	if serverConfig == nil {
		return nil
	}

	securitySchemes := serverConfig.SecuritySchemes
	securities := req.RawRequest.Security
	if req.RawRequest.Security.IsEmpty() && serverConfig.Security != nil {
		securities = serverConfig.Security
	}

	if securities.IsOptional() || len(securitySchemes) == 0 {
		return nil
	}

	for _, security := range securities {
		sc, ok := securitySchemes[security.Name()]
		if !ok {
			continue
		}

		hasAuth, err := req.applySecurityScheme(sc, isExplain)
		if hasAuth || err != nil {
			return err
		}
	}

	return nil
}

func (req *RetryableRequest) applySecurityScheme(securityScheme rest.SecurityScheme, isExplain bool) (bool, error) {
	if securityScheme.SecuritySchemer == nil {
		return false, nil
	}

	if req.Headers == nil {
		req.Headers = http.Header{}
	}

	switch config := securityScheme.SecuritySchemer.(type) {
	case *rest.BasicAuthConfig:
		username := config.GetUsername()
		password := config.GetPassword()
		if config.Header != "" {
			b64Value := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			req.Headers.Set(rest.AuthorizationHeader, "Basic "+b64Value)
		} else {
			req.URL.User = url.UserPassword(username, password)
		}

		return true, nil
	case *rest.HTTPAuthConfig:
		headerName := config.Header
		if headerName == "" {
			headerName = rest.AuthorizationHeader
		}
		scheme := config.Scheme
		if scheme == "bearer" {
			scheme = "Bearer"
		}
		v := config.GetValue()
		if v != "" {
			req.Headers.Set(headerName, fmt.Sprintf("%s %s", scheme, eitherMaskSecret(v, isExplain)))

			return true, nil
		}
	case *rest.APIKeyAuthConfig:
		switch config.In {
		case rest.APIKeyInHeader:
			value := config.GetValue()
			if value != "" {
				req.Headers.Set(config.Name, eitherMaskSecret(value, isExplain))

				return true, nil
			}
		case rest.APIKeyInQuery:
			value := config.GetValue()
			if value != "" {
				endpoint := req.URL
				q := endpoint.Query()
				q.Add(config.Name, eitherMaskSecret(value, isExplain))
				endpoint.RawQuery = q.Encode()
				req.URL = endpoint

				return true, nil
			}
		case rest.APIKeyInCookie:
			// Cookie header should be forwarded from Hasura engine
			return true, nil
		default:
			return false, fmt.Errorf("unsupported location for apiKey scheme: %s", config.In)
		}
	// TODO: support OAuth and OIDC
	// Authentication headers can be forwarded from Hasura engine
	case *rest.OAuth2Config, *rest.OpenIDConnectConfig:
	case *rest.CookieAuthConfig:
		return true, nil
	case *rest.MutualTLSAuthConfig:
		// the server may require not only mutualTLS authentication
		return false, nil
	default:
		return false, fmt.Errorf("unsupported security scheme: %s", securityScheme.GetType())
	}

	return false, nil
}

func (req *RetryableRequest) applySettings(settings *rest.NDCHttpSettings, isExplain bool) error {
	if settings == nil {
		return nil
	}
	serverConfig := req.getServerConfig(settings)
	if serverConfig == nil {
		return nil
	}
	if err := req.applySecurity(serverConfig, isExplain); err != nil {
		return err
	}

	req.applyDefaultHeaders(serverConfig.GetHeaders())
	req.applyDefaultHeaders(settings.GetHeaders())

	return nil
}

func (req *RetryableRequest) applyDefaultHeaders(defaultHeaders map[string]string) {
	for k, envValue := range defaultHeaders {
		if req.Headers.Get(k) != "" {
			continue
		}
		if envValue != "" {
			req.Headers.Set(k, envValue)
		}
	}
}
