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
)

type oas3SchemaBuilder struct {
	builder   *OAS3Builder
	apiPath   string
	location  rest.ParameterLocation
	writeMode bool
}

func newOAS3SchemaBuilder(builder *OAS3Builder, apiPath string, location rest.ParameterLocation, writeMode bool) *oas3SchemaBuilder {
	return &oas3SchemaBuilder{
		builder:   builder,
		apiPath:   apiPath,
		writeMode: writeMode,
		location:  location,
	}
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas3SchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (*SchemaInfoCache, error) {
	if schemaProxy == nil {
		return nil, errParameterSchemaEmpty(fieldPaths)
	}

	innerSchema := schemaProxy.Schema()
	if innerSchema == nil {
		return nil, fmt.Errorf("cannot get schema of $.%s from proxy: %s", strings.Join(fieldPaths, "."), schemaProxy.GetReference())
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
		refName := getSchemaRefTypeNameV3(rawRefName)
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

// get and convert an OpenAPI data type to a NDC type
func (oc *oas3SchemaBuilder) getSchemaType(typeSchema *base.Schema, fieldPaths []string) (*SchemaInfoCache, error) {
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

	if typeSchema.AdditionalProperties != nil && (typeSchema.AdditionalProperties.B || typeSchema.AdditionalProperties.A != nil) {
		return &SchemaInfoCache{
			TypeRead:   oc.builder.buildScalarJSON(),
			TypeWrite:  oc.builder.buildScalarJSON(),
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
		}, nil
	}

	if len(typeSchema.Type) == 0 {
		if oc.builder.Strict {
			return nil, errParameterSchemaEmpty(fieldPaths)
		}

		var result schema.TypeEncoder = oc.builder.buildScalarJSON()
		if typeSchema.Nullable != nil && *typeSchema.Nullable {
			result = schema.NewNullableType(result)
		}

		return &SchemaInfoCache{
			TypeRead:   result,
			TypeWrite:  result,
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
		}, nil
	}

	if len(typeSchema.Type) > 1 || isPrimitiveScalar(typeSchema.Type) {
		scalarName, nullable := getScalarFromType(oc.builder.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.builder.trimPathPrefix(oc.apiPath), fieldPaths)
		var resultType schema.TypeEncoder = schema.NewNamedType(scalarName)
		if nullable || (typeSchema.Nullable != nil && *typeSchema.Nullable) {
			resultType = schema.NewNullableType(resultType)
		}

		return &SchemaInfoCache{
			TypeRead:   resultType,
			TypeWrite:  resultType,
			TypeSchema: createSchemaFromOpenAPISchema(typeSchema),
		}, nil
	}

	var typeResult *SchemaInfoCache
	typeName := typeSchema.Type[0]
	switch typeName {
	case "object":
		return oc.evalObjectType(typeSchema, false, fieldPaths)
	case "array":
		var itemSchema *SchemaInfoCache
		var err error
		nullable := (typeSchema.Nullable != nil && *typeSchema.Nullable)
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

		if nullable {
			typeResult.TypeRead = schema.NewNullableType(typeResult.TypeRead)
			typeResult.TypeWrite = schema.NewNullableType(typeResult.TypeWrite)
		}

		return typeResult, nil
	default:
		return nil, fmt.Errorf("unsupported schema type %s", typeName)
	}
}

func (oc *oas3SchemaBuilder) evalObjectType(baseSchema *base.Schema, forcePropertiesNullable bool, fieldPaths []string) (*SchemaInfoCache, error) {
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

	readObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    typeResult.XML,
	}
	writeObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    typeResult.XML,
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

		switch {
		case !propResult.TypeSchema.ReadOnly && !propResult.TypeSchema.WriteOnly:
			readObject.Fields[propName] = readField
			writeObject.Fields[propName] = writeField
		case !oc.writeMode && propResult.TypeSchema.ReadOnly:
			readObject.Fields[propName] = readField
		default:
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

// Support converting oneOf, allOf or anyOf to object types with merge strategy
func (oc *oas3SchemaBuilder) buildUnionSchemaType(baseSchema *base.Schema, schemaProxies []*base.SchemaProxy, unionType oasUnionType, fieldPaths []string) (*SchemaInfoCache, error) {
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
		if len(baseSchema.Type) > 1 || isPrimitiveScalar(baseSchema.Type) {
			scalarName, nullable := getScalarFromType(oc.builder.schema, baseSchema.Type, baseSchema.Format, baseSchema.Enum, oc.builder.trimPathPrefix(oc.apiPath), fieldPaths)
			var result schema.TypeEncoder = schema.NewNamedType(scalarName)
			if nullable {
				result = schema.NewNullableType(result)
			}

			return &SchemaInfoCache{
				TypeRead:   result,
				TypeWrite:  result,
				TypeSchema: createSchemaFromOpenAPISchema(baseSchema),
			}, nil
		}

		if len(baseSchema.Type) == 1 && baseSchema.Type[0] == "object" {
			return oc.evalObjectType(baseSchema, true, fieldPaths)
		}

		var result schema.TypeEncoder = schema.NewNamedType(string(rest.ScalarJSON))
		if nullable {
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
		schemaResult, err := newOAS3SchemaBuilder(oc.builder, oc.apiPath, oc.location, false).
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

	if oc.writeMode && len(writeObject.Fields) > 0 {
		refName = writeRefName
	}

	return &SchemaInfoCache{
		TypeRead:   schema.NewNamedType(refName),
		TypeWrite:  schema.NewNamedType(writeRefName),
		TypeSchema: typeSchema,
	}, nil
}
