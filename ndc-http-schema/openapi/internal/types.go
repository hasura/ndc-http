package internal

import (
	"errors"
	"log/slog"
	"regexp"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

var (
	bracketRegexp    = regexp.MustCompile(`[\{\}]`)
	oasVariableRegex = regexp.MustCompile(`^\{([a-zA-Z0-9_-]+)\}$`)
)

var errParameterNameRequired = errors.New("parameter name is empty")

var preferredContentTypes = []string{rest.ContentTypeJSON, rest.ContentTypeXML}

var defaultScalarTypes = map[rest.ScalarName]schema.ScalarType{
	rest.ScalarBoolean: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationBoolean().Encode(),
	},
	rest.ScalarString: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	rest.ScalarInt32: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationInt32().Encode(),
	},
	rest.ScalarInt64: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationInt64().Encode(),
	},
	rest.ScalarFloat32: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationFloat32().Encode(),
	},
	rest.ScalarFloat64: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationFloat64().Encode(),
	},
	rest.ScalarJSON: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationJSON().Encode(),
	},
	// string format variants https://swagger.io/docs/specification/data-models/data-types/#string
	// string with date format
	rest.ScalarDate: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationDate().Encode(),
	},
	// string with date-time format
	rest.ScalarTimestampTZ: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationTimestampTZ().Encode(),
	},
	// string with byte format
	rest.ScalarBytes: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationBytes().Encode(),
	},
	// string with byte format
	rest.ScalarBinary: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationBytes().Encode(),
	},
	rest.ScalarEmail: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	rest.ScalarURI: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	rest.ScalarUUID: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationUUID().Encode(),
	},
	rest.ScalarIPV4: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	rest.ScalarIPV6: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	// unix-time the timestamp integer which is measured in seconds since the Unix epoch
	rest.ScalarUnixTime: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		ExtractionFunctions: schema.ScalarTypeExtractionFunctions{},
		Representation:      schema.NewTypeRepresentationInt32().Encode(),
	},
}

var typeRepresentationToScalarNameRelationship = map[schema.TypeRepresentationType]rest.ScalarName{
	schema.TypeRepresentationTypeBoolean:     rest.ScalarBoolean,
	schema.TypeRepresentationTypeString:      rest.ScalarString,
	schema.TypeRepresentationTypeInt8:        rest.ScalarInt32,
	schema.TypeRepresentationTypeInt16:       rest.ScalarInt32,
	schema.TypeRepresentationTypeInt32:       rest.ScalarInt32,
	schema.TypeRepresentationTypeInt64:       rest.ScalarInt64,
	schema.TypeRepresentationTypeBigInteger:  rest.ScalarBigDecimal,
	schema.TypeRepresentationTypeBigDecimal:  rest.ScalarBigDecimal,
	schema.TypeRepresentationTypeBytes:       rest.ScalarBytes,
	schema.TypeRepresentationTypeDate:        rest.ScalarDate,
	schema.TypeRepresentationTypeEnum:        rest.ScalarString,
	schema.TypeRepresentationTypeFloat32:     rest.ScalarFloat32,
	schema.TypeRepresentationTypeFloat64:     rest.ScalarFloat64,
	schema.TypeRepresentationTypeGeography:   rest.ScalarJSON,
	schema.TypeRepresentationTypeGeometry:    rest.ScalarJSON,
	schema.TypeRepresentationTypeTimestamp:   rest.ScalarTimestampTZ,
	schema.TypeRepresentationTypeTimestampTZ: rest.ScalarTimestampTZ,
	schema.TypeRepresentationTypeJSON:        rest.ScalarJSON,
}

var integerTypeRepresentations = []schema.TypeRepresentationType{
	schema.TypeRepresentationTypeInt32,
	schema.TypeRepresentationTypeInt64,
	schema.TypeRepresentationTypeInt8,
	schema.TypeRepresentationTypeInt16,
}

var floatTypeRepresentations = []schema.TypeRepresentationType{
	schema.TypeRepresentationTypeFloat32,
	schema.TypeRepresentationTypeFloat64,
}

var stringTypeRepresentations = []schema.TypeRepresentationType{
	schema.TypeRepresentationTypeString,
	schema.TypeRepresentationTypeEnum,
	schema.TypeRepresentationTypeDate,
	schema.TypeRepresentationTypeTimestamp,
	schema.TypeRepresentationTypeTimestampTZ,
}

const xmlValueFieldName string = "xmlValue"

var xmlValueField = rest.ObjectField{
	ObjectField: schema.ObjectField{
		Description: utils.ToPtr("Value of the xml field"),
		Type:        schema.NewNamedType(string(rest.ScalarString)).Encode(),
	},
	HTTP: &rest.TypeSchema{
		Type: []string{"string"},
		XML: &rest.XMLSchema{
			Text: true,
		},
	},
}

// ConvertOptions represent the common convert options for both OpenAPI v2 and v3.
type ConvertOptions struct {
	MethodAlias         map[string]string
	AllowedContentTypes []string
	Prefix              string
	TrimPrefix          string
	EnvPrefix           string
	NoDeprecation       bool
	Logger              *slog.Logger
}

type oasUnionType string

const (
	oasOneOf oasUnionType = "oneOf"
	oasAnyOf oasUnionType = "anyOf"
	oasAllOf oasUnionType = "allOf"
)
