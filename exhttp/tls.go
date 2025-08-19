package exhttp

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hasura/ndc-sdk-go/v2/utils"
)

var systemCertPool = x509.SystemCertPool

// We should avoid that users unknowingly use a vulnerable TLS version.
// The defaults should be a safe configuration.
const defaultMinTLSVersion = tls.VersionTLS12

// Uses the default MaxVersion from "crypto/tls" which is the maximum supported version.
const defaultMaxTLSVersion = 0

var tlsVersions = map[string]uint16{
	"1.0": tls.VersionTLS10,
	"1.1": tls.VersionTLS11,
	"1.2": tls.VersionTLS12,
	"1.3": tls.VersionTLS13,
}

// NewTLSTransport creates a new HTTP transport with TLS configuration.
func NewTLSTransport(
	baseTransport http.RoundTripper,
	tlsConfig *TLSConfig,
	logger *slog.Logger,
) (*http.Transport, error) {
	bTransport, ok := baseTransport.(*http.Transport)
	if !ok {
		bTransport, _ = http.DefaultTransport.(*http.Transport)
	}

	tlsCfg, err := loadTLSConfig(tlsConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	transport := bTransport.Clone()
	transport.TLSClientConfig = tlsCfg

	return transport, nil
}

// TLSConfig represents the transport layer security (LTS) configuration for the mutualTLS authentication.
type TLSConfig struct {
	// Path to the TLS cert to use for TLS required connections.
	CertFile *utils.EnvString `json:"certFile,omitempty"                 mapstructure:"certFile"                 yaml:"certFile,omitempty"`
	// Alternative to cert_file. Provide the certificate contents as a base64-encoded string instead of a filepath.
	CertPem *utils.EnvString `json:"certPem,omitempty"                  mapstructure:"certPem"                  yaml:"certPem,omitempty"`
	// Path to the TLS key to use for TLS required connections.
	KeyFile *utils.EnvString `json:"keyFile,omitempty"                  mapstructure:"keyFile"                  yaml:"keyFile,omitempty"`
	// Alternative to key_file. Provide the key contents as a base64-encoded string instead of a filepath.
	KeyPem *utils.EnvString `json:"keyPem,omitempty"                   mapstructure:"keyPem"                   yaml:"keyPem,omitempty"`
	// Path to the CA cert. For a client this verifies the server certificate. For a server this verifies client certificates.
	// If empty uses system root CA.
	CAFile *utils.EnvString `json:"caFile,omitempty"                   mapstructure:"caFile"                   yaml:"caFile,omitempty"`
	// Alternative to ca_file. Provide the CA cert contents as a base64-encoded string instead of a filepath.
	CAPem *utils.EnvString `json:"caPem,omitempty"                    mapstructure:"caPem"                    yaml:"caPem,omitempty"`
	// Additionally you can configure TLS to be enabled but skip verifying the server's certificate chain.
	InsecureSkipVerify *utils.EnvBool `json:"insecureSkipVerify,omitempty"       mapstructure:"insecureSkipVerify"       yaml:"insecureSkipVerify,omitempty"`
	// Whether to load the system certificate authorities pool alongside the certificate authority.
	IncludeSystemCACertsPool *utils.EnvBool `json:"includeSystemCACertsPool,omitempty" mapstructure:"includeSystemCACertsPool" yaml:"includeSystemCACertsPool,omitempty"`
	// Minimum acceptable TLS version.
	MinVersion string `json:"minVersion,omitempty"               mapstructure:"minVersion"               yaml:"minVersion,omitempty"`
	// Maximum acceptable TLS version.
	MaxVersion string `json:"maxVersion,omitempty"               mapstructure:"maxVersion"               yaml:"maxVersion,omitempty"`
	// Explicit cipher suites can be set. If left blank, a safe default list is used.
	// See https://go.dev/src/crypto/tls/cipher_suites.go for a list of supported cipher suites.
	CipherSuites []string `json:"cipherSuites,omitempty"             mapstructure:"cipherSuites"             yaml:"cipherSuites,omitempty"`
	// ServerName requested by client for virtual hosting.
	// This sets the ServerName in the TLSConfig. Please refer to
	// https://godoc.org/crypto/tls#Config for more information. (optional)
	ServerName *utils.EnvString `json:"serverName,omitempty"               mapstructure:"serverName"               yaml:"serverName,omitempty"`
}

// Validate if the current instance is valid.
func (tc TLSConfig) Validate() error {
	minTLS, err := tc.GetMinVersion()
	if err != nil {
		return fmt.Errorf("TLSConfig.minVersion: %w", err)
	}

	maxTLS, err := tc.GetMaxVersion()
	if err != nil {
		return fmt.Errorf("TLSConfig.maxVersion: %w", err)
	}

	if maxTLS < minTLS && maxTLS != defaultMaxTLSVersion {
		return errors.New(
			"invalid TLS configuration: minVersion cannot be greater than max_version",
		)
	}

	if tc.CAFile != nil && tc.CAPem != nil {
		caFile, err := tc.CAFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caFile: %w", err)
		}

		caPem, err := tc.CAFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caPem: %w", err)
		}

		if caFile != "" && caPem != "" {
			return errors.New(
				"invalid TLS configuration: provide either a CA file or the PEM-encoded string, but not both",
			)
		}
	}

	if tc.CertFile != nil && tc.CertPem != nil {
		certFile, err := tc.CertFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.certFile: %w", err)
		}

		certPem, err := tc.CertPem.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caPem: %w", err)
		}

		if certFile != "" && certPem != "" {
			return errors.New(
				"for auth via TLS, provide either a certificate or the PEM-encoded string, but not both",
			)
		}
	}

	if tc.KeyFile != nil && tc.KeyPem != nil {
		keyFile, err := tc.KeyFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.keyFile: %w", err)
		}

		keyPem, err := tc.KeyPem.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.keyPem: %w", err)
		}

		if keyFile != "" && keyPem != "" {
			return errors.New(
				"for auth via TLS, provide either a certificate or the PEM-encoded string, but not both",
			)
		}
	}

	if tc.IncludeSystemCACertsPool != nil {
		_, err := tc.IncludeSystemCACertsPool.GetOrDefault(false)
		if err != nil {
			return err
		}
	}

	if tc.ServerName != nil {
		_, err := tc.ServerName.GetOrDefault("")
		if err != nil {
			return err
		}
	}

	return nil
}

// GetMinVersion parses the minx TLS version from string.
func (tc TLSConfig) GetMinVersion() (uint16, error) {
	return tc.convertTLSVersion(tc.MinVersion, defaultMinTLSVersion)
}

// GetMaxVersion parses the max TLS version from string.
func (tc TLSConfig) GetMaxVersion() (uint16, error) {
	return tc.convertTLSVersion(tc.MinVersion, defaultMaxTLSVersion)
}

func (tc TLSConfig) convertTLSVersion(v string, defaultVersion uint16) (uint16, error) {
	// Use a default that is explicitly defined
	if v == "" {
		return defaultVersion, nil
	}

	val, ok := tlsVersions[v]
	if !ok {
		return 0, fmt.Errorf("unsupported TLS version: %q", v)
	}

	return val, nil
}

// loadTLSConfig loads TLS certificates and returns a tls.Config.
// This will set the RootCAs and Certificates of a tls.Config.
func loadTLSConfig(tlsConfig *TLSConfig, logger *slog.Logger) (*tls.Config, error) {
	certPool, err := loadCACertPool(tlsConfig)
	if err != nil {
		return nil, err
	}

	minTLS, err := tlsConfig.GetMinVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS min_version: %w", err)
	}

	maxTLS, err := tlsConfig.GetMaxVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS max_version: %w", err)
	}

	cipherSuites, err := convertCipherSuites(tlsConfig.CipherSuites)
	if err != nil {
		return nil, err
	}

	var serverName string
	if tlsConfig.ServerName != nil {
		serverName, err = tlsConfig.ServerName.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS server name: %w", err)
		}
	}

	var insecureSkipVerify bool

	if tlsConfig.InsecureSkipVerify != nil {
		insecureSkipVerify, err = tlsConfig.InsecureSkipVerify.GetOrDefault(false)
		if err != nil {
			return nil, fmt.Errorf("failed to parse insecureSkipVerify: %w", err)
		}
	}

	cert, err := loadCertificate(tlsConfig, insecureSkipVerify, logger)
	if err != nil {
		return nil, err
	}

	var certificates []tls.Certificate

	if cert != nil {
		certificates = append(certificates, *cert)
	} else if !insecureSkipVerify {
		return nil, nil
	}

	result := &tls.Config{
		RootCAs:            certPool,
		Certificates:       certificates,
		MinVersion:         minTLS,
		MaxVersion:         maxTLS,
		CipherSuites:       cipherSuites,
		ServerName:         serverName,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	}

	return result, nil
}

func loadCACertPool(tlsConfig *TLSConfig) (*x509.CertPool, error) {
	// There is no need to load the System Certs for RootCAs because
	// if the value is nil, it will default to checking against th System Certs.
	var err error

	var certPool *x509.CertPool

	var includeSystemCACertsPool bool

	if tlsConfig.IncludeSystemCACertsPool != nil {
		includeSystemCACertsPool, err = tlsConfig.IncludeSystemCACertsPool.GetOrDefault(false)
		if err != nil {
			return nil, fmt.Errorf("invalid includeSystemCACertsPool config: %w", err)
		}
	}

	if tlsConfig.CAPem != nil {
		caPem, err := tlsConfig.CAPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load CA CertPool PEM: %w", err)
		}

		if caPem != "" {
			caData, err := base64.StdEncoding.DecodeString(caPem)
			if err != nil {
				return nil, fmt.Errorf("failed to decode CA PEM from base64: %w", err)
			}

			return loadCertPem(caData, includeSystemCACertsPool)
		}
	}

	if tlsConfig.CAFile != nil {
		caFile, err := tlsConfig.CAFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load CA CertPool File: %w", err)
		}

		if caFile != "" {
			return loadCertFile(caFile, includeSystemCACertsPool)
		}
	}

	return certPool, nil
}

func loadCertFile(certPath string, includeSystemCACertsPool bool) (*x509.CertPool, error) {
	certPem, err := os.ReadFile(filepath.Clean(certPath))
	if err != nil {
		return nil, fmt.Errorf("failed to load cert %s: %w", certPath, err)
	}

	return loadCertPem(certPem, includeSystemCACertsPool)
}

func loadCertPem(certPem []byte, includeSystemCACertsPool bool) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()

	if includeSystemCACertsPool {
		scp, err := systemCertPool()
		if err != nil {
			return nil, err
		}

		if scp != nil {
			certPool = scp
		}
	}

	if !certPool.AppendCertsFromPEM(certPem) {
		return nil, errors.New("failed to parse cert")
	}

	return certPool, nil
}

func loadCertificate(
	tlsConfig *TLSConfig,
	insecureSkipVerify bool,
	logger *slog.Logger,
) (*tls.Certificate, error) {
	var certData, keyData []byte

	var certPem, keyPem string

	var err error

	if tlsConfig.CertPem != nil {
		certPem, err = tlsConfig.CertPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate PEM: %w", err)
		}
	}

	if certPem != "" {
		certData, err = base64.StdEncoding.DecodeString(certPem)
		if err != nil {
			return nil, fmt.Errorf("failed to decode certificate PEM from base64: %w", err)
		}
	} else if tlsConfig.CertFile != nil {
		certFile, err := tlsConfig.CertFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate file: %w", err)
		}

		if certFile != "" {
			certData, err = os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read certificate file: %w", err)
			}
		}
	}

	if len(certData) == 0 && !insecureSkipVerify {
		logger.Warn("both certificate PEM and file are empty")
	}

	if tlsConfig.KeyPem != nil {
		keyPem, err = tlsConfig.KeyPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load key PEM: %w", err)
		}
	}

	if keyPem != "" {
		keyData, err = base64.StdEncoding.DecodeString(keyPem)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key PEM from base64: %w", err)
		}
	} else if tlsConfig.KeyFile != nil {
		keyFile, err := tlsConfig.KeyFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load key file: %w", err)
		}

		if keyFile != "" {
			keyData, err = os.ReadFile(keyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read key file: %w", err)
			}
		}
	}

	if len(keyData) == 0 && !insecureSkipVerify {
		logger.Warn("both key PEM and file are empty")
	}

	if len(keyData) == 0 && len(certData) == 0 {
		return nil, nil
	}

	if len(keyData) == 0 || len(certData) == 0 {
		return nil, errors.New("provide both certificate and key, or neither")
	}

	certificate, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS cert and key PEMs: %w", err)
	}

	return &certificate, err
}

func convertCipherSuites(cipherSuites []string) ([]uint16, error) {
	var result []uint16

	var errs []error

	for _, suite := range cipherSuites {
		found := false

		for _, supported := range tls.CipherSuites() {
			if suite == supported.Name {
				result = append(result, supported.ID)
				found = true

				break
			}
		}

		if !found {
			errs = append(errs, fmt.Errorf("invalid TLS cipher suite: %q", suite))
		}
	}

	return result, errors.Join(errs...)
}
