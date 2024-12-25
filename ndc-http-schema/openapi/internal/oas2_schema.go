package internal

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
)

type oas2SchemaBuilder struct {
	builder  *OAS2Builder
	apiPath  string
	location rest.ParameterLocation
}

func newOAS2SchemaBuilder(builder *OAS2Builder, apiPath string, location rest.ParameterLocation) *oas2SchemaBuilder {
	return &oas2SchemaBuilder{
		builder:  builder,
		apiPath:  apiPath,
		location: location,
	}
}

// get and convert an OpenAPI data type to a NDC type from parameter
func (oc *oas2SchemaBuilder) getSchemaTypeFromParameter(param *v2.Parameter, fieldPaths []string) (schema.TypeEncoder, error) {
	var typeEncoder schema.TypeEncoder
	nullable := param.Required == nil || !*param.Required

	switch param.Type {
	case "object":
		return nil, fmt.Errorf("%s: unsupported object parameter", strings.Join(fieldPaths, "."))
	case "array":
		if param.Items == nil || param.Items.Type == "" {
			if oc.builder.Strict {
				return nil, fmt.Errorf("%s: array item is empty", strings.Join(fieldPaths, "."))
			}

			typeEncoder = schema.NewArrayType(oc.builder.buildScalarJSON())
		} else {
			itemName := getScalarFromType(oc.builder.schema, []string{param.Items.Type}, param.Format, param.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			typeEncoder = schema.NewArrayType(schema.NewNamedType(itemName))
		}
	default:
		if !isPrimitiveScalar([]string{param.Type}) {
			return nil, fmt.Errorf("%s: unsupported schema type %s", strings.Join(fieldPaths, "."), param.Type)
		}

		scalarName := getScalarFromType(oc.builder.schema, []string{param.Type}, param.Format, param.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
		typeEncoder = schema.NewNamedType(scalarName)
	}

	if nullable {
		return schema.NewNullableType(typeEncoder), nil
	}

	return typeEncoder, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas2SchemaBuilder) getSchemaType(typeSchema *base.Schema, fieldPaths []string) (*SchemaInfoCache, error) {
	if typeSchema == nil {
		return nil, errParameterSchemaEmpty(fieldPaths)
	}

	if oc.builder.ConvertOptions.NoDeprecation && typeSchema.Deprecated != nil && *typeSchema.Deprecated {
		return nil, nil
	}

	if len(typeSchema.AllOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.AllOf, oasAllOf, fieldPaths)
	}

	if len(typeSchema.AnyOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.AnyOf, oasAnyOf, fieldPaths)
	}

	if len(typeSchema.OneOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.OneOf, oasOneOf, fieldPaths)
	}

	oasTypes, nullable := extractNullableFromOASTypes(typeSchema.Type)
	nullable = nullable || (typeSchema.Nullable != nil && *typeSchema.Nullable)
	if len(oasTypes) == 0 || (typeSchema.AdditionalProperties != nil && (typeSchema.AdditionalProperties.B || typeSchema.AdditionalProperties.A != nil)) {
		if len(typeSchema.Type) == 0 && oc.builder.Strict {
			return nil, errParameterSchemaEmpty(fieldPaths)
		}

		var result schema.TypeEncoder = oc.builder.buildScalarJSON()
		if nullable {
			result = schema.NewNullableType(result)
		}

		return &SchemaInfoCache{
			TypeRead:   result,
			TypeWrite:  result,
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
		}, nil
	}

	if len(oasTypes) > 1 || isPrimitiveScalar(oasTypes) {
		scalarName := getScalarFromType(oc.builder.schema, oasTypes, typeSchema.Format, typeSchema.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
		var resultType schema.TypeEncoder = schema.NewNamedType(scalarName)
		if nullable {
			resultType = schema.NewNullableType(resultType)
		}

		return &SchemaInfoCache{
			TypeRead:   resultType,
			TypeWrite:  resultType,
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
		}, nil
	}

	var typeResult *SchemaInfoCache
	var err error
	typeName := oasTypes[0]
	switch typeName {
	case "object":
		typeResult, err = oc.evalObjectType(typeSchema, false, fieldPaths)
		if err != nil {
			return nil, err
		}
	case "array":
		var itemSchema *SchemaInfoCache
		if typeSchema.Items == nil || typeSchema.Items.A == nil {
			if oc.builder.Strict {
				return nil, fmt.Errorf("%s: array item is empty", strings.Join(fieldPaths, "."))
			}
		} else {
			itemSchema, err = oc.getSchemaTypeFromProxy(typeSchema.Items.A, false, fieldPaths)
			if err != nil {
				return nil, err
			}
		}

		if itemSchema == nil {
			itemSchema = &SchemaInfoCache{
				TypeRead:  schema.NewNullableType(oc.builder.buildScalarJSON()),
				TypeWrite: schema.NewNullableType(oc.builder.buildScalarJSON()),
			}
		}

		typeResult = &SchemaInfoCache{
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
			TypeRead:   schema.NewArrayType(itemSchema.TypeRead),
			TypeWrite:  schema.NewArrayType(itemSchema.TypeWrite),
		}

		if itemSchema.TypeSchema != nil {
			typeResult.TypeSchema.Items = itemSchema.TypeSchema
		}

	default:
		return nil, fmt.Errorf("unsupported schema type %s", typeName)
	}

	if nullable {
		typeResult.TypeRead = schema.NewNullableType(typeResult.TypeRead)
		typeResult.TypeWrite = schema.NewNullableType(typeResult.TypeWrite)
	}

	return typeResult, nil
}

func (oc *oas2SchemaBuilder) evalObjectType(baseSchema *base.Schema, forcePropertiesNullable bool, fieldPaths []string) (*SchemaInfoCache, error) {
	typeResult := createSchemaFromOpenAPISchema(baseSchema)
	refName := utils.StringSliceToPascalCase(fieldPaths)

	if baseSchema.Properties == nil || baseSchema.Properties.IsZero() {
		// treat no-property objects as a JSON scalar
		var scalarType schema.TypeEncoder = oc.builder.buildScalarJSON()
		if baseSchema.Nullable != nil && *baseSchema.Nullable {
			scalarType = schema.NewNullableType(scalarType)
		}

		return &SchemaInfoCache{
			TypeRead:   scalarType,
			TypeWrite:  scalarType,
			TypeSchema: typeResult,
		}, nil
	}

	xmlSchema := typeResult.XML
	if xmlSchema == nil {
		xmlSchema = &rest.XMLSchema{}
	}

	if xmlSchema.Name == "" {
		xmlSchema.Name = fieldPaths[0]
	}

	readObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    xmlSchema,
	}
	writeObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    xmlSchema,
	}

	if typeResult.Description != "" {
		readObject.Description = &typeResult.Description
		writeObject.Description = &typeResult.Description
	}

	for prop := baseSchema.Properties.First(); prop != nil; prop = prop.Next() {
		propName := prop.Key()
		oc.builder.Logger.Debug(
			"property",
			slog.String("name", propName),
			slog.Any("field", fieldPaths))

		nullable := forcePropertiesNullable || !slices.Contains(baseSchema.Required, propName)
		propResult, err := oc.getSchemaTypeFromProxy(prop.Value(), nullable, append(fieldPaths, propName))
		if err != nil {
			return nil, err
		}

		if propResult == nil || propResult.TypeRead == nil {
			continue
		}

		if propResult.TypeSchema == nil {
			propResult.TypeSchema = &rest.TypeSchema{
				Type: []string{},
			}
		}

		readField := rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: propResult.TypeRead.Encode(),
			},
			HTTP: propResult.TypeSchema,
		}

		writeField := rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: propResult.TypeWrite.Encode(),
			},
			HTTP: propResult.TypeSchema,
		}

		if propResult.TypeSchema.Description != "" {
			readField.Description = &propResult.TypeSchema.Description
			writeField.Description = &propResult.TypeSchema.Description
		}

		readObject.Fields[propName] = readField
		if !propResult.TypeSchema.ReadOnly {
			writeObject.Fields[propName] = writeField
		}
	}

	writeRefName := formatWriteObjectName(refName)
	if isXMLLeafObject(readObject) {
		readObject.Fields[xmlValueFieldName] = xmlValueField
	}

	if isXMLLeafObject(writeObject) {
		writeObject.Fields[xmlValueFieldName] = xmlValueField
	}

	oc.builder.schema.ObjectTypes[refName] = readObject
	oc.builder.schema.ObjectTypes[writeRefName] = writeObject

	result := &SchemaInfoCache{
		TypeRead:   schema.NewNamedType(refName),
		TypeWrite:  schema.NewNamedType(writeRefName),
		TypeSchema: typeResult,
	}

	if baseSchema.Nullable != nil && *baseSchema.Nullable {
		result.TypeRead = schema.NewNullableType(result.TypeRead)
		result.TypeWrite = schema.NewNullableType(result.TypeWrite)
	}

	return result, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas2SchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (*SchemaInfoCache, error) {
	if schemaProxy == nil {
		return nil, errParameterSchemaEmpty(fieldPaths)
	}

	innerSchema := schemaProxy.Schema()
	if innerSchema == nil {
		return nil, fmt.Errorf("cannot get schema from proxy: %s", schemaProxy.GetReference())
	}

	var result *SchemaInfoCache
	var err error

	rawRefName := schemaProxy.GetReference()
	if rawRefName == "" {
		result, err = oc.getSchemaType(innerSchema, fieldPaths)
		if err != nil {
			return nil, err
		}
	} else if typeCache, ok := oc.builder.schemaCache[rawRefName]; ok {
		result = &typeCache
	} else {
		// return early object from ref
		refName := getSchemaRefTypeNameV2(rawRefName)
		readSchemaName := utils.ToPascalCase(refName)
		writeSchemaName := formatWriteObjectName(readSchemaName)

		oc.builder.schemaCache[rawRefName] = SchemaInfoCache{
			TypeRead:  schema.NewNamedType(readSchemaName),
			TypeWrite: schema.NewNamedType(writeSchemaName),
		}

		result, err = oc.getSchemaType(innerSchema, []string{refName})
		if err != nil {
			return nil, err
		}

		oc.builder.schemaCache[rawRefName] = *result
	}

	if result == nil || result.TypeRead == nil {
		return nil, nil
	}

	if nullable {
		if !isNullableType(result.TypeRead) {
			result.TypeRead = schema.NewNullableType(result.TypeRead)
		}

		if !isNullableType(result.TypeWrite) {
			result.TypeWrite = schema.NewNullableType(result.TypeWrite)
		}
	}

	return result, nil
}

// Support converting allOf and anyOf to object types with merge strategy
func (oc *oas2SchemaBuilder) buildUnionSchemaType(baseSchema *base.Schema, schemaProxies []*base.SchemaProxy, unionType oasUnionType, fieldPaths []string) (*SchemaInfoCache, error) {
	proxies, mergedType, isNullable := evalSchemaProxiesSlice(schemaProxies, oc.location)
	nullable := isNullable || (baseSchema.Nullable != nil && *baseSchema.Nullable)
	if mergedType != nil {
		result, err := oc.getSchemaType(mergedType, fieldPaths)
		if err != nil {
			return nil, err
		}
		if result != nil && result.TypeSchema != nil && result.TypeSchema.Description == "" && baseSchema.Description != "" {
			result.TypeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
		}

		return result, nil
	}

	switch len(proxies) {
	case 0:
		oasTypes, isNullable := extractNullableFromOASTypes(baseSchema.Type)
		if len(baseSchema.Type) > 1 || isPrimitiveScalar(baseSchema.Type) {
			scalarName := getScalarFromType(oc.builder.schema, oasTypes, baseSchema.Format, baseSchema.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			var result schema.TypeEncoder = schema.NewNamedType(scalarName)
			if nullable || isNullable {
				result = schema.NewNullableType(result)
			}

			return &SchemaInfoCache{
				TypeRead:   result,
				TypeWrite:  result,
				TypeSchema: createSchemaFromOpenAPISchema(baseSchema),
			}, nil
		}

		if len(oasTypes) == 1 && baseSchema.Type[0] == "object" {
			schemaResult, err := oc.evalObjectType(baseSchema, true, fieldPaths)
			if err != nil {
				return nil, err
			}

			if nullable || isNullable {
				schemaResult.TypeRead = schema.NewNullableType(schemaResult.TypeRead)
				schemaResult.TypeWrite = schema.NewNullableType(schemaResult.TypeWrite)
			}

			return schemaResult, nil
		}

		var result schema.TypeEncoder = schema.NewNamedType(string(rest.ScalarJSON))
		if nullable || isNullable {
			result = schema.NewNullableType(result)
		}

		return &SchemaInfoCache{
			TypeRead:   result,
			TypeWrite:  result,
			TypeSchema: createSchemaFromOpenAPISchema(baseSchema),
		}, nil
	case 1:
		result, err := oc.getSchemaTypeFromProxy(proxies[0], nullable, fieldPaths)
		if err != nil {
			return nil, err
		}

		if result == nil {
			return nil, nil
		}

		if result.TypeSchema != nil && result.TypeSchema.Description == "" && baseSchema.Description != "" {
			result.TypeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
		}

		return result, nil
	}

	typeSchema := &rest.TypeSchema{
		Type: []string{"object"},
	}

	if baseSchema.Description != "" {
		typeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
	}

	var readObjectItems []rest.ObjectType
	var writeObjectItems []rest.ObjectType

	for i, item := range proxies {
		schemaResult, err := newOAS2SchemaBuilder(oc.builder, oc.apiPath, oc.location).
			getSchemaTypeFromProxy(item, nullable, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, err
		}

		var readObj rest.ObjectType
		name := getNamedType(schemaResult.TypeRead, false, "")
		isObject := name != "" && !isPrimitiveScalar(schemaResult.TypeSchema.Type) && !slices.Contains(schemaResult.TypeSchema.Type, "array")
		if isObject {
			readObj, isObject = oc.builder.schema.ObjectTypes[name]
			if isObject {
				readObjectItems = append(readObjectItems, readObj)
			}
		}

		if !isObject {
			schemaResult.TypeSchema = &rest.TypeSchema{
				Description: typeSchema.Description,
				Type:        []string{},
			}

			jsonScalar := oc.builder.buildScalarJSON()

			return &SchemaInfoCache{
				TypeRead:   jsonScalar,
				TypeWrite:  jsonScalar,
				TypeSchema: schemaResult.TypeSchema,
			}, nil
		}

		writeName := formatWriteObjectName(name)
		writeObj, ok := oc.builder.schema.ObjectTypes[writeName]
		if !ok {
			writeObj = readObj
		}

		writeObjectItems = append(writeObjectItems, writeObj)
	}

	readObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	writeObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}

	if baseSchema.Description != "" {
		readObject.Description = &baseSchema.Description
		writeObject.Description = &baseSchema.Description
	}

	if err := mergeUnionObjects(oc.builder.schema, &readObject, readObjectItems, unionType, fieldPaths); err != nil {
		return nil, err
	}

	if err := mergeUnionObjects(oc.builder.schema, &writeObject, writeObjectItems, unionType, fieldPaths); err != nil {
		return nil, err
	}

	refName := utils.ToPascalCase(strings.Join(fieldPaths, " "))
	writeRefName := formatWriteObjectName(refName)
	if len(readObject.Fields) > 0 {
		oc.builder.schema.ObjectTypes[refName] = readObject
	}
	if len(writeObject.Fields) > 0 {
		oc.builder.schema.ObjectTypes[writeRefName] = writeObject
	}

	return &SchemaInfoCache{
		TypeRead:   schema.NewNamedType(refName),
		TypeWrite:  schema.NewNamedType(writeRefName),
		TypeSchema: typeSchema,
	}, nil
}

func (oc *oas2SchemaBuilder) trimPathPrefix(input string) string {
	if oc.builder.ConvertOptions.TrimPrefix == "" {
		return input
	}

	return strings.TrimPrefix(input, oc.builder.ConvertOptions.TrimPrefix)
}
