# Authentication

The current version supports API key and Auth token authentication schemes. The configuration is inspired from `securitySchemes` [with env variables](https://github.com/hasura/ndc-http/ndc-http-schema#authentication). The connector supports the following authentication strategies:

- API Key
- Bearer Auth
- Cookie
- OAuth 2.0
- Mutual TLS

The configuration automatically generates environment variables for those security schemes.

## OAuth 2.0

For other OAuth 2.0 flows, you need to enable [headers forwarding](#headers-forwarding) from the Hasura engine to the connector.

## Cookie

For Cookie authentication and OAuth 2.0, you need to enable [headers forwarding](#headers-forwarding) from the Hasura engine to the connector.

## Headers Forwarding

Enable `forwardHeaders` in the configuration file.

```yaml
# ...
forwardHeaders:
  enabled: true
  argumentField: headers
```

And configure in the connector link metadata.

```yaml
kind: DataConnectorLink
version: v1
definition:
  name: my_api
  # ...
  argumentPresets:
    - argument: headers
      value:
        httpHeaders:
          forward:
            - Cookie
          additional: {}
```

See the configuration example in [Hasura docs](https://hasura.io/docs/3.0/recipes/business-logic/http-header-forwarding/#step-2-update-the-metadata-1).

## Mutual TLS

If the `mutualTLS` security scheme exists the TLS configuration will be generated in the `settings` field.

```yaml
settings:
  servers:
    - url:
        env: PET_STORE_URL
  securitySchemes:
    mtls:
      type: mutualTLS
  tls:
    # Path to the TLS cert to use for TLS required connections.
    certFile:
      env: PET_STORE_CERT_FILE
    # Alternative to cert_file. Provide the certificate contents as a base64-encoded string instead of a filepath.
    certPem:
      env: PET_STORE_CERT_PEM
    # Path to the TLS key to use for TLS required connections.
    keyFile:
      env: PET_STORE_KEY_FILE
    # Alternative to key_file. Provide the key contents as a base64-encoded string instead of a filepath.
    keyPem:
      env: PET_STORE_KEY_PEM
    # Path to the CA cert.
    caFile:
      env: PET_STORE_CA_FILE
    # Alternative to ca_file. Provide the CA cert contents as a base64-encoded string instead of a filepath.
    caPem:
      env: PET_STORE_CA_PEM
    # Additionally you can configure TLS to be enabled but skip verifying the server's certificate chain (optional).
    insecureSkipVerify:
      env: PET_STORE_INSECURE_SKIP_VERIFY
      value: false
    # Whether to load the system certificate authorities pool alongside the certificate authority (optional).
    includeSystemCACertsPool:
      env: PET_STORE_INCLUDE_SYSTEM_CA_CERT_POOL
      value: false
    # ServerName requested by client for virtual hosting (optional).
    serverName:
      env: PET_STORE_SERVER_NAME
    # Minimum acceptable TLS version (optional).
    minVersion: "1.0"
    # Maximum acceptable TLS version (optional).
    maxVersion: "1.3"
    # Explicit cipher suites can be set. If left blank, a safe default list is used (optional).
    cipherSuites:
      - TLS_AES_128_GCM_SHA256
```

You can configure either file path `*_FILE` or inline PEM data `*_PEM` in bases64-encoded string.

If the service has many servers, you can configure different TLS configuration for each server. However, you need to [manually patch the configuration](../README.md#json-patch):

```yaml
settings:
  servers:
    - url:
        env: PET_STORE_URL
    - url:
        env: PET_STORE_URL_2
      tls:
        certFile:
          env: PET_STORE_CERT_FILE_2
        # ...
  securitySchemes:
    mtls:
      type: mutualTLS
  tls:
    certFile:
      env: PET_STORE_CERT_FILE
    # ...
```