module github.com/hasura/ndc-http/ndc-http-schema

go 1.23.0

toolchain go1.23.4

require (
	github.com/alecthomas/kong v1.10.0
	github.com/evanphx/json-patch v0.5.2
	github.com/google/go-cmp v0.7.0
	github.com/hasura/ndc-http/exhttp v0.0.1
	github.com/hasura/ndc-sdk-go v1.9.1
	github.com/invopop/jsonschema v0.13.0
	github.com/lmittmann/tint v1.0.7
	github.com/pb33f/libopenapi v0.21.10
	github.com/theory/jsonpath v0.4.1
	github.com/wk8/go-ordered-map/v2 v2.1.9-0.20240815153524-6ea36470d1bd
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.2
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/speakeasy-api/jsonpath v0.6.1 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/hasura/ndc-http/exhttp => ../exhttp
