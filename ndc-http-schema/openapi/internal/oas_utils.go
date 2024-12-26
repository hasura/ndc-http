package internal

import (
	"slices"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"gopkg.in/yaml.v3"
)

func getSchemaRefTypeNameV2(name string) string {
	result := schemaRefNameV2Regexp.FindStringSubmatch(name)
	if len(result) < 2 {
		return ""
	}

	return result[1]
}

func getSchemaRefTypeNameV3(name string) string {
	result := schemaRefNameV3Regexp.FindStringSubmatch(name)
	if len(result) < 2 {
		return ""
	}

	return result[1]
}

func extractNullableFromOASTypes(names []string) ([]string, bool) {
	var typeNames []string
	var nullable bool

	for _, name := range names {
		if name == "null" {
			nullable = true
		} else {
			typeNames = append(typeNames, name)
		}
	}

	return typeNames, nullable
}

func getScalarFromType(sm *rest.NDCHttpSchema, names []string, format string, enumNodes []*yaml.Node, fieldPaths []string) string {
	var scalarName string
	var scalarType *schema.ScalarType

	namesLen := len(names)
	switch {
	case namesLen == 0 && len(enumNodes) > 0:
		scalarName, scalarType = buildEnumScalar(sm, enumNodes, fieldPaths)
	case namesLen == 1:
		scalarName, scalarType = getScalarFromOASType(sm, names, format, enumNodes, fieldPaths)
	default:
		scalarName = string(rest.ScalarJSON)
		scalarType = defaultScalarTypes[rest.ScalarJSON]
	}

	sm.AddScalar(scalarName, *scalarType)

	return scalarName
}

func buildEnumScalar(sm *rest.NDCHttpSchema, enumNodes []*yaml.Node, fieldPaths []string) (string, *schema.ScalarType) {
	enums := make([]string, len(enumNodes))
	for i, enum := range enumNodes {
		if enum.Value == "null" {
			continue
		}

		enums[i] = enum.Value
	}

	scalarType := schema.NewScalarType()
	scalarType.Representation = schema.NewTypeRepresentationEnum(enums).Encode()

	scalarName := utils.StringSliceToPascalCase(fieldPaths)
	if canSetEnumToSchema(sm, scalarName, enums) {
		return scalarName, scalarType
	}

	// if the name exists, add enum above name with Enum suffix
	scalarName += "Enum"

	return scalarName, scalarType
}

func getScalarFromOASType(sm *rest.NDCHttpSchema, names []string, format string, enumNodes []*yaml.Node, fieldPaths []string) (string, *schema.ScalarType) {
	var scalarName string
	var scalarType *schema.ScalarType

	switch names[0] {
	case "boolean":
		scalarName = string(rest.ScalarBoolean)
		scalarType = defaultScalarTypes[rest.ScalarBoolean]
	case "integer":
		switch format {
		case "unix-time":
			scalarName = string(rest.ScalarUnixTime)
			scalarType = defaultScalarTypes[rest.ScalarUnixTime]
		case "int64":
			scalarName = string(rest.ScalarInt64)
			scalarType = defaultScalarTypes[rest.ScalarInt64]
		default:
			scalarName = string(rest.ScalarInt32)
			scalarType = defaultScalarTypes[rest.ScalarInt32]
		}
	case "long":
		scalarName = string(rest.ScalarInt64)
		scalarType = defaultScalarTypes[rest.ScalarInt64]
	case "number":
		switch format {
		case "float":
			scalarName = string(rest.ScalarFloat32)
			scalarType = defaultScalarTypes[rest.ScalarFloat32]
		default:
			scalarName = string(rest.ScalarFloat64)
			scalarType = defaultScalarTypes[rest.ScalarFloat64]
		}
	case "file":
		scalarName = string(rest.ScalarBinary)
		scalarType = defaultScalarTypes[rest.ScalarBinary]
	case "string":
		if len(enumNodes) > 0 {
			return buildEnumScalar(sm, enumNodes, fieldPaths)
		}

		switch format {
		case "date":
			scalarName = string(rest.ScalarDate)
			scalarType = defaultScalarTypes[rest.ScalarDate]
		case "date-time":
			scalarName = string(rest.ScalarTimestampTZ)
			scalarType = defaultScalarTypes[rest.ScalarTimestampTZ]
		case "byte", "base64":
			scalarName = string(rest.ScalarBytes)
			scalarType = defaultScalarTypes[rest.ScalarBytes]
		case "binary":
			scalarName = string(rest.ScalarBinary)
			scalarType = defaultScalarTypes[rest.ScalarBinary]
		case "uuid":
			scalarName = string(rest.ScalarUUID)
			scalarType = defaultScalarTypes[rest.ScalarUUID]
		case "uri":
			scalarName = string(rest.ScalarURI)
			scalarType = defaultScalarTypes[rest.ScalarURI]
		case "ipv4":
			scalarName = string(rest.ScalarIPV4)
			scalarType = defaultScalarTypes[rest.ScalarIPV4]
		case "ipv6":
			scalarName = string(rest.ScalarIPV6)
			scalarType = defaultScalarTypes[rest.ScalarIPV6]
		default:
			scalarName = string(rest.ScalarString)
			scalarType = defaultScalarTypes[rest.ScalarString]
		}
	default:
		scalarName = string(rest.ScalarJSON)
		scalarType = defaultScalarTypes[rest.ScalarJSON]
	}

	return scalarName, scalarType
}

func canSetEnumToSchema(sm *rest.NDCHttpSchema, scalarName string, enums []string) bool {
	existedScalar, ok := sm.ScalarTypes[scalarName]
	if !ok {
		return true
	}

	existedEnum, err := existedScalar.Representation.AsEnum()
	if err == nil && utils.SliceUnorderedEqual(enums, existedEnum.OneOf) {
		return true
	}

	return false
}

// remove nullable types from raw OpenAPI types
func evaluateOpenAPITypes(input []string) []string {
	var typeNames []string
	for _, t := range input {
		if t != "null" {
			typeNames = append(typeNames, t)
		}
	}

	return typeNames
}

func createSchemaFromOpenAPISchema(input *base.Schema) *rest.TypeSchema {
	ps := &rest.TypeSchema{
		Type: []string{},
	}
	if input == nil {
		return ps
	}
	ps.Type = evaluateOpenAPITypes(input.Type)
	ps.Format = input.Format
	ps.Pattern = utils.RemoveYAMLSpecialCharacters([]byte(input.Pattern))
	ps.Maximum = input.Maximum
	ps.Minimum = input.Minimum
	ps.MaxLength = input.MaxLength
	ps.MinLength = input.MinLength
	ps.Description = utils.StripHTMLTags(input.Description)
	ps.ReadOnly = input.ReadOnly != nil && *input.ReadOnly
	ps.WriteOnly = input.WriteOnly != nil && *input.WriteOnly

	if input.XML != nil {
		ps.XML = &rest.XMLSchema{
			Name:      input.XML.Name,
			Prefix:    input.XML.Prefix,
			Namespace: input.XML.Namespace,
			Wrapped:   input.XML.Wrapped,
			Attribute: input.XML.Attribute,
		}
	}

	return ps
}

// check if the OAS type is a scalar
func isPrimitiveScalar(names []string) bool {
	for _, name := range names {
		if !slices.Contains([]string{"boolean", "integer", "number", "string", "file", "long", "null"}, name) {
			return false
		}
	}

	return true
}

// Find common fields in all objects to merge the type.
// If they have the same type, we don't need to wrap it with the nullable type.
func mergeUnionObjects(httpSchema *rest.NDCHttpSchema, dest *rest.ObjectType, srcObjects []rest.ObjectType, unionType oasUnionType, fieldPaths []string) {
	mergedObjectFields := make(map[string][]rest.ObjectField)
	for _, object := range srcObjects {
		for key, field := range object.Fields {
			mergedObjectFields[key] = append(mergedObjectFields[key], field)
		}
	}

	for key, fields := range mergedObjectFields {
		if len(fields) == 1 {
			newField := rest.ObjectField{
				ObjectField: schema.ObjectField{
					Description: fields[0].Description,
					Arguments:   fields[0].Arguments,
					Type:        fields[0].Type,
				},
				HTTP: fields[0].HTTP,
			}

			if unionType != oasAllOf && !isNullableType(newField.Type.Interface()) {
				newField.Type = (schema.NullableType{
					Type:           schema.TypeNullable,
					UnderlyingType: newField.Type,
				}).Encode()
			}

			dest.Fields[key] = newField

			continue
		}

		var unionField rest.ObjectField
		for i, field := range fields {
			if i == 0 {
				unionField = field

				continue
			}

			unionType, ok := mergeUnionTypes(httpSchema, field.Type, unionField.Type, append(fieldPaths, key))
			unionField.Type = unionType.Encode()
			if !ok {
				break
			}

			if unionField.Description == nil && field.Description != nil {
				unionField.Description = field.Description
			}

			if unionField.HTTP == nil && field.HTTP != nil {
				unionField.HTTP = field.HTTP
			}
		}

		if len(fields) < len(srcObjects) && unionType != oasAllOf && !isNullableType(unionField.Type.Interface()) {
			unionField.Type = (schema.NullableType{
				Type:           schema.TypeNullable,
				UnderlyingType: unionField.Type,
			}).Encode()
		}

		dest.Fields[key] = unionField
	}
}

// evaluate and filter invalid types in allOf, anyOf or oneOf schemas
func evalSchemaProxiesSlice(schemaProxies []*base.SchemaProxy, location rest.ParameterLocation) ([]*base.SchemaProxy, *base.Schema, bool, bool) {
	var results []*base.SchemaProxy
	var typeNames []string
	var nullable, isEmptyObject bool

	for _, proxy := range schemaProxies {
		if proxy == nil {
			continue
		}
		sc := proxy.Schema()
		if sc == nil || (len(sc.Type) == 0 && len(sc.AllOf) == 0 && len(sc.AnyOf) == 0 && len(sc.OneOf) == 0) {
			continue
		}

		// empty string enum is considered as nullable, e.g. key1=&key2=
		// however, it's redundant and prevents the tool converting correct types
		if (len(sc.Type) == 1 && sc.Type[0] == "null") ||
			(location == rest.InQuery && (sc.Type[0] == "string" && len(sc.Enum) == 1 && (sc.Enum[0] == nil || sc.Enum[0].Value == ""))) {
			nullable = true

			continue
		}

		if len(sc.Type) == 1 && sc.Type[0] == "object" && (sc.Properties == nil || sc.Properties.Len() == 0) &&
			(sc.AdditionalProperties == nil || !sc.AdditionalProperties.B) {
			isEmptyObject = true

			continue
		}

		results = append(results, proxy)
		if len(sc.Type) == 0 {
			typeNames = append(typeNames, "any")
		} else if !slices.Contains(typeNames, sc.Type[0]) {
			typeNames = append(typeNames, sc.Type[0])
		}
	}

	if len(typeNames) == 1 && len(results) > 1 && typeNames[0] == "string" {
		// if the anyOf array contains both string and enum
		// we can cast them to string
		return nil, &base.Schema{
			Type: typeNames,
		}, nullable, isEmptyObject
	}

	return results, nil, nullable, isEmptyObject
}

// guess the result type from content type
func getResultTypeFromContentType(httpSchema *rest.NDCHttpSchema, contentType string) schema.TypeEncoder {
	var scalarName rest.ScalarName
	switch {
	case strings.HasPrefix(contentType, "text/"):
		scalarName = rest.ScalarString
	case contentType == rest.ContentTypeOctetStream || strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "video/"):
		scalarName = rest.ScalarBinary
	default:
		scalarName = rest.ScalarJSON
	}

	httpSchema.AddScalar(string(scalarName), *defaultScalarTypes[scalarName])

	return schema.NewNamedType(string(scalarName))
}

func guessScalarResultTypeFromContentType(contentType string) rest.ScalarName {
	ct := strings.TrimSpace(strings.Split(contentType, ";")[0])
	switch {
	case utils.IsContentTypeJSON(ct) || utils.IsContentTypeXML(ct) || ct == rest.ContentTypeNdJSON:
		return rest.ScalarJSON
	case utils.IsContentTypeText(ct):
		return rest.ScalarString
	default:
		return rest.ScalarBinary
	}
}
