package exhttp

import (
	"log/slog"
	"testing"
	"time"

	"github.com/hasura/ndc-sdk-go/v2/utils"
	"github.com/prometheus/common/model"
	"gotest.tools/v3/assert"
)

func TestHTTPTransport(t *testing.T) {
	baseConfig := HTTPTransportConfig{
		Dialer: &DialerConfig{
			Timeout:           utils.ToPtr(model.Duration(time.Second)),
			KeepAliveEnabled:  utils.ToPtr(true),
			KeepAliveInterval: utils.ToPtr(model.Duration(time.Minute)),
			KeepAliveCount:    utils.ToPtr(uint(1)),
			KeepAliveIdle:     utils.ToPtr(model.Duration(15 * time.Second)),
		},
		IdleConnTimeout:        utils.ToPtr(model.Duration(10 * time.Second)),
		ResponseHeaderTimeout:  utils.ToPtr(model.Duration(11 * time.Second)),
		TLSHandshakeTimeout:    utils.ToPtr(model.Duration(12 * time.Second)),
		ExpectContinueTimeout:  utils.ToPtr(model.Duration(13 * time.Second)),
		MaxIdleConns:           utils.ToPtr(10),
		MaxIdleConnsPerHost:    utils.ToPtr(9),
		MaxConnsPerHost:        utils.ToPtr(8),
		MaxResponseHeaderBytes: utils.ToPtr(int64(3000)),
		ReadBufferSize:         utils.ToPtr(2000),
		WriteBufferSize:        utils.ToPtr(1000),
	}
	tlsConfig := &TLSConfig{
		InsecureSkipVerify: utils.ToPtr(utils.NewEnvBoolValue(true)),
	}

	_ = NewTelemetryTransport(baseConfig.ToTransport(), TelemetryConfig{})
	_, err := HTTPTransportTLSConfig{
		HTTPTransportConfig: baseConfig,
		TLS:                 tlsConfig,
	}.ToTransport(slog.Default())
	assert.NilError(t, err)

	_, err = NewTLSTransport(nil, tlsConfig, slog.Default())
	assert.NilError(t, err)

	_, err = NewTLSTransport(nil, &TLSConfig{
		CertFile: &utils.EnvString{
			Value: utils.ToPtr("foo.pem"),
		},
	}, slog.Default())
	assert.ErrorContains(
		t,
		err,
		"failed to load TLS config: failed to read certificate file: open foo.pem: no such file or directory",
	)
}
