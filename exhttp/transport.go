package exhttp

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/common/model"
)

// DialerConfig contains options the http.Dialer to connect to an address.
type DialerConfig struct {
	// The maximum amount of time a dial will wait for a connect to complete.
	// If Deadline is also set, it may fail earlier.
	Timeout *model.Duration `json:"timeout,omitempty" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$" mapstructure:"timeout" yaml:"timeout"`
	// Keep-alive probes are enabled by default.
	KeepAliveEnabled *bool `json:"keepAliveEnabled,omitempty" mapstructure:"keepAliveEnabled" yaml:"keepAliveEnabled"`
	// The time between keep-alive probes. If zero, a default value of 15 seconds is used.
	KeepAliveInterval *model.Duration `json:"keepAliveInterval,omitempty" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$" mapstructure:"keepAliveInterval" yaml:"keepAliveInterval"`
	// The maximum number of keep-alive probes that can go unanswered before dropping a connection.
	// If zero, a default value of 9 is used.
	KeepAliveCount *uint `json:"keepAliveCount,omitempty" jsonschema:"nullable,min=0" mapstructure:"keepAliveCount" yaml:"keepAliveCount"`
	// The time that the connection must be idle before the first keep-alive probe is sent.
	// If zero, a default value of 15 seconds is used.
	KeepAliveIdle *model.Duration `json:"keepAliveIdle,omitempty" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$" mapstructure:"keepAliveIdle" yaml:"keepAliveIdle"`
}

// HTTPTransportConfig stores the http.Transport configuration for the http client.
type HTTPTransportConfig struct {
	// Options the http.Dialer to connect to an address
	Dialer *DialerConfig `json:"dialer,omitempty" mapstructure:"dialer" yaml:"dialer"`
	// The maximum amount of time an idle (keep-alive) connection will remain idle before closing itself. Zero means no limit.
	IdleConnTimeout *model.Duration `json:"idleConnTimeout,omitempty" mapstructure:"idleConnTimeout" yaml:"idleConnTimeout" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$"`
	// If non-zero, specifies the amount of time to wait for a server's response headers after fully writing the request (including its body, if any).
	// This time does not include the time to read the response body.
	// This timeout is used to cover cases where the tcp connection works but the server never answers.
	ResponseHeaderTimeout *model.Duration `json:"responseHeaderTimeout,omitempty" mapstructure:"responseHeaderTimeout" yaml:"responseHeaderTimeout" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$"`
	// The maximum amount of time to wait for a TLS handshake. Zero means no timeout.
	TLSHandshakeTimeout *model.Duration `json:"tlsHandshakeTimeout,omitempty" mapstructure:"tlsHandshakeTimeout" yaml:"tlsHandshakeTimeout" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$"`
	// If non-zero, specifies the amount of time to wait for a server's first response headers after fully writing the request headers if the request has an "Expect: 100-continue" header.
	ExpectContinueTimeout *model.Duration `json:"expectContinueTimeout,omitempty" mapstructure:"expectContinueTimeout" yaml:"expectContinueTimeout" jsonschema:"nullable,type=string,pattern=^((([0-9]+h)?([0-9]+m)?([0-9]+s))|(([0-9]+h)?([0-9]+m))|([0-9]+h))$"`
	// MaxIdleConns controls the maximum number of idle (keep-alive) connections across all hosts. Zero means no limit.
	MaxIdleConns *int `json:"maxIdleConns,omitempty" mapstructure:"maxIdleConns" yaml:"maxIdleConns" jsonschema:"nullable,min=0"`
	// MaxIdleConnsPerHost, if non-zero, controls the maximum idle (keep-alive) connections to keep per-host.
	MaxIdleConnsPerHost *int `json:"maxIdleConnsPerHost,omitempty" mapstructure:"maxIdleConnsPerHost" yaml:"maxIdleConnsPerHost" jsonschema:"nullable,min=0"`
	// Optionally limits the total number of connections per host, including connections in the dialing, active, and idle states.
	// On limit violation, dials will block. Zero means no limit.
	MaxConnsPerHost *int `json:"maxConnsPerHost,omitempty" mapstructure:"maxConnsPerHost" yaml:"maxConnsPerHost" jsonschema:"nullable,min=0"`
	// Specifies a limit on how many response bytes are allowed in the server's response header.
	// Zero means to use a default limit.
	MaxResponseHeaderBytes *int64 `json:"maxResponseHeaderBytes,omitempty" mapstructure:"maxResponseHeaderBytes" yaml:"maxResponseHeaderBytes" jsonschema:"nullable,min=0"`
	// ReadBufferSize specifies the size of the read buffer used when reading from the transport.
	// If zero, a default (currently 4KB) is used.
	ReadBufferSize *int `json:"readBufferSize,omitempty" mapstructure:"readBufferSize" yaml:"readBufferSize" jsonschema:"nullable,min=0"`
	// WriteBufferSize specifies the size of the write buffer used when writing to the transport.
	// If zero, a default (currently 4KB) is used.
	WriteBufferSize *int `json:"writeBufferSize,omitempty" mapstructure:"writeBufferSize" yaml:"writeBufferSize" jsonschema:"nullable,min=0"`
}

// ToTransport creates an http transport from the configuration.
func (ttc HTTPTransportConfig) ToTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Interval: 30 * time.Second,
		},
	}

	if ttc.Dialer != nil {
		if ttc.Dialer.Timeout != nil {
			dialer.Timeout = time.Duration(*ttc.Dialer.Timeout)
		}

		if ttc.Dialer.KeepAliveEnabled != nil {
			dialer.KeepAliveConfig.Enable = *ttc.Dialer.KeepAliveEnabled
		}

		if ttc.Dialer.KeepAliveCount != nil {
			dialer.KeepAliveConfig.Count = int(*ttc.Dialer.KeepAliveCount)
		}

		if ttc.Dialer.KeepAliveIdle != nil {
			dialer.KeepAliveConfig.Idle = time.Duration(*ttc.Dialer.KeepAliveIdle)
		}

		if ttc.Dialer.KeepAliveInterval != nil {
			dialer.KeepAliveConfig.Interval = time.Duration(*ttc.Dialer.KeepAliveInterval)
		}
	}

	defaultTransport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   16,
		ResponseHeaderTimeout: time.Minute,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if ttc.ExpectContinueTimeout != nil {
		defaultTransport.ExpectContinueTimeout = time.Duration(*ttc.ExpectContinueTimeout)
	}

	if ttc.IdleConnTimeout != nil {
		defaultTransport.IdleConnTimeout = time.Duration(*ttc.IdleConnTimeout)
	}

	if ttc.MaxConnsPerHost != nil {
		defaultTransport.MaxConnsPerHost = *ttc.MaxConnsPerHost
	}

	if ttc.MaxIdleConns != nil {
		defaultTransport.MaxIdleConns = *ttc.MaxIdleConns
	}

	if ttc.MaxIdleConnsPerHost != nil {
		defaultTransport.MaxIdleConnsPerHost = *ttc.MaxIdleConnsPerHost
	}

	if ttc.ResponseHeaderTimeout != nil {
		defaultTransport.ResponseHeaderTimeout = time.Duration(*ttc.ResponseHeaderTimeout)
	}

	if ttc.TLSHandshakeTimeout != nil {
		defaultTransport.TLSHandshakeTimeout = time.Duration(*ttc.TLSHandshakeTimeout)
	}

	if ttc.MaxResponseHeaderBytes != nil && *ttc.MaxResponseHeaderBytes > 0 {
		defaultTransport.MaxResponseHeaderBytes = *ttc.MaxResponseHeaderBytes
	}

	if ttc.ReadBufferSize != nil && *ttc.ReadBufferSize > 0 {
		defaultTransport.ReadBufferSize = *ttc.ReadBufferSize
	}

	if ttc.WriteBufferSize != nil && *ttc.WriteBufferSize > 0 {
		defaultTransport.WriteBufferSize = *ttc.WriteBufferSize
	}

	return defaultTransport
}

// HTTPTransportTLSConfig stores the http.Transport configuration for the http client with TLS.
type HTTPTransportTLSConfig struct {
	HTTPTransportConfig

	TLS *TLSConfig `json:"tls,omitempty" jsonschema:"nullable" mapstructure:"tls" yaml:"tls"`
}

// ToTransport creates an http transport from the configuration with TLS.
func (hc HTTPTransportTLSConfig) ToTransport(logger *slog.Logger) (*http.Transport, error) {
	transport := hc.HTTPTransportConfig.ToTransport()

	if hc.TLS != nil {
		tls, err := loadTLSConfig(hc.TLS, logger)
		if err != nil {
			return nil, err
		}

		transport.TLSClientConfig = tls
	}

	return transport, nil
}
