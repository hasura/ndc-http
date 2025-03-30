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

// OASBuilderState the common state for the OpenAPI to HTTP schema builder.
type OASBuilderState struct {
	*ConvertOptions

	schema *rest.NDCHttpSchema
	// stores prebuilt and evaluating information of component schema types.
	// some undefined schema types aren't stored in either object nor scalar,
	// or self-reference types that haven't added into the object_types map yet.
	// This cache temporarily stores them to avoid infinite recursive references.
	schemaCache map[string]SchemaInfoCache
}

// NewOASBuilderState creates an OASBuilderState instance.
func NewOASBuilderState(options ConvertOptions) *OASBuilderState {
	builder := &OASBuilderState{
		schema:         rest.NewNDCHttpSchema(),
		schemaCache:    make(map[string]SchemaInfoCache),
		ConvertOptions: applyConvertOptions(options),
	}

	for key, scalar := range defaultScalarTypes {
		builder.schema.ScalarTypes[string(key)] = scalar
	}

	return builder
}

type oasSchemaBuilder struct {
	state    *OASBuilderState
	apiPath  string
	location rest.ParameterLocation
}

func newOASSchemaBuilder(state *OASBuilderState, apiPath string, location rest.ParameterLocation) *oasSchemaBuilder {
	return &oasSchemaBuilder{
		state:    state,
		apiPath:  apiPath,
		location: location,
	}
}

// get and convert an OpenAPI data type to a NDC type.
func (oc *oasSchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (*SchemaInfoCache, error) {
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
	} else if typeCache, ok := oc.state.schemaCache[rawRefName]; ok {
		result = &typeCache
	} else {
		// return early object from ref
		refName := getSchemaRefTypeName(rawRefName)
		readSchemaName := utils.ToPascalCase(refName)
		writeSchemaName := formatWriteObjectName(readSchemaName)

		oc.state.schemaCache[rawRefName] = SchemaInfoCache{
			TypeRead:  schema.NewNamedType(readSchemaName),
			TypeWrite: schema.NewNamedType(writeSchemaName),
		}

		result, err = oc.getSchemaType(innerSchema, []string{refName})
		if err != nil {
			return nil, err
		}

		oc.state.schemaCache[rawRefName] = *result
	}

	if result == nil || result.TypeRead == nil {
		return nil, nil
	}

	if nullable {
		result.TypeRead = utils.WrapNullableTypeEncoder(result.TypeRead)
		result.TypeWrite = utils.WrapNullableTypeEncoder(result.TypeWrite)
	}

	return result, nil
}

// get and convert an OpenAPI data type to a NDC type.
func (oc *oasSchemaBuilder) getSchemaType(baseSchema *base.Schema, fieldPaths []string) (*SchemaInfoCache, error) {
	if baseSchema == nil {
		return nil, errParameterSchemaEmpty(fieldPaths)
	}

	if oc.state.ConvertOptions.NoDeprecation && baseSchema.Deprecated != nil && *baseSchema.Deprecated {
		return nil, nil
	}

	if len(baseSchema.AllOf) > 0 {
		return oc.buildUnionSchemaType(baseSchema, baseSchema.AllOf, oasAllOf, fieldPaths)
	}

	if len(baseSchema.AnyOf) > 0 {
		return oc.buildUnionSchemaType(baseSchema, baseSchema.AnyOf, oasAnyOf, fieldPaths)
	}

	if len(baseSchema.OneOf) > 0 {
		return oc.buildUnionSchemaType(baseSchema, baseSchema.OneOf, oasOneOf, fieldPaths)
	}

	oasTypes, nullable := extractNullableFromOASTypes(baseSchema.Type)
	nullable = nullable || (baseSchema.Nullable != nil && *baseSchema.Nullable)

	if len(oasTypes) == 0 {
		// if the OAS schema has properties or items we can consider this type as an object or array
		if baseSchema.Properties != nil && baseSchema.Properties.Len() > 0 {
			oasTypes = []string{"object"}
		} else if baseSchema.Items != nil && baseSchema.Items.A != nil {
			oasTypes = []string{"array"}
		}
	}

	if len(oasTypes) != 1 || isPrimitiveScalar(oasTypes) {
		scalarName := getScalarFromType(oc.state.schema, oasTypes, baseSchema.Format, baseSchema.Enum, fieldPaths)
		var resultType schema.TypeEncoder = schema.NewNamedType(scalarName)

		if nullable {
			resultType = schema.NewNullableType(resultType)
		}

		return &SchemaInfoCache{
			TypeRead:   resultType,
			TypeWrite:  resultType,
			TypeSchema: createSchemaFromOpenAPISchema(baseSchema),
		}, nil
	}

	var typeResult *SchemaInfoCache
	var err error
	typeName := oasTypes[0]

	switch typeName {
	case "object":
		typeResult, err = oc.evalObjectType(baseSchema, fieldPaths)
		if err != nil {
			return nil, err
		}
	case "array":
		var itemSchema *SchemaInfoCache
		if baseSchema.Items != nil && baseSchema.Items.A != nil {
			itemSchema, err = oc.getSchemaTypeFromProxy(baseSchema.Items.A, false, fieldPaths)
			if err != nil {
				return nil, err
			}
		}

		if itemSchema == nil {
			scalarType := schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON)))
			itemSchema = &SchemaInfoCache{
				TypeRead:  scalarType,
				TypeWrite: scalarType,
			}
		}

		typeResult = &SchemaInfoCache{
			TypeSchema: createSchemaFromOpenAPISchema(baseSchema),
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
		typeResult.TypeRead = utils.WrapNullableTypeEncoder(typeResult.TypeRead)
		typeResult.TypeWrite = utils.WrapNullableTypeEncoder(typeResult.TypeWrite)
	}

	return typeResult, nil
}

func (oc *oasSchemaBuilder) evalObjectType(baseSchema *base.Schema, fieldPaths []string) (*SchemaInfoCache, error) {
	typeResult := createSchemaFromOpenAPISchema(baseSchema)
	refName := utils.StringSliceToPascalCase(fieldPaths)
	writeRefName := formatWriteObjectName(refName)

	if baseSchema.Properties == nil || baseSchema.Properties.IsZero() || (baseSchema.AdditionalProperties != nil && (baseSchema.AdditionalProperties.B || baseSchema.AdditionalProperties.A != nil)) {
		// treat no-property objects as a JSON scalar
		var scalarType schema.TypeEncoder = schema.NewNamedType(string(rest.ScalarJSON))
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
	readObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    xmlSchema,
	}

	writeObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    xmlSchema,
	}

	// assume that the object type is a root field,
	// get the openapi schema name as the alias.
	if len(fieldPaths) == 1 {
		if refName != fieldPaths[0] {
			readObject.Alias = fieldPaths[0]
		}

		if writeRefName != fieldPaths[0] {
			writeObject.Alias = fieldPaths[0]
		}
	} else {
		refName = utils.BuildUniqueSchemaTypeName(oc.state.schema, refName+"Object")
		writeRefName = utils.BuildUniqueSchemaTypeName(oc.state.schema, formatWriteObjectName(refName))
	}

	if typeResult.Description != "" {
		readObject.Description = &typeResult.Description
		writeObject.Description = &typeResult.Description
	}

	for prop := baseSchema.Properties.First(); prop != nil; prop = prop.Next() {
		propName := prop.Key()
		oc.state.Logger.Debug(
			"property",
			slog.String("name", propName),
			slog.Any("field", fieldPaths))
		nullable := !slices.Contains(baseSchema.Required, propName)
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
		case propResult.TypeSchema.ReadOnly:
			readObject.Fields[propName] = readField
		default:
			writeObject.Fields[propName] = writeField
		}
	}

	if isXMLLeafObject(readObject) {
		readObject.Fields[xmlValueFieldName] = xmlValueField
	}

	if isXMLLeafObject(writeObject) {
		writeObject.Fields[xmlValueFieldName] = xmlValueField
	}

	oc.state.schema.ObjectTypes[refName] = readObject
	oc.state.schema.ObjectTypes[writeRefName] = writeObject

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

// Support converting oneOf, allOf or anyOf to object types with merge strategy.
func (oc *oasSchemaBuilder) buildUnionSchemaType(baseSchema *base.Schema, schemaProxies []*base.SchemaProxy, unionType oasUnionType, fieldPaths []string) (*SchemaInfoCache, error) {
	// only evaluate field paths for anonymous types
	if len(fieldPaths) > 1 {
		fieldPaths = append(fieldPaths, string(unionType))
	}

	proxies, mergedType, isNullable, isEmptyObject := evalSchemaProxiesSlice(schemaProxies, oc.location)
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
		if isEmptyObject {
			return createSchemaInfoJSONScalar(nullable), nil
		}

		oasTypes, isNullable := extractNullableFromOASTypes(baseSchema.Type)

		if len(baseSchema.Type) > 1 || isPrimitiveScalar(baseSchema.Type) {
			scalarName := getScalarFromType(oc.state.schema, oasTypes, baseSchema.Format, baseSchema.Enum, fieldPaths)
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

		if len(oasTypes) == 1 && (baseSchema.Type[0] == "object" || (baseSchema.Properties != nil && baseSchema.Properties.Len() > 0)) {
			schemaResult, err := oc.evalObjectType(baseSchema, fieldPaths)
			if err != nil {
				return nil, err
			}

			if nullable || isNullable {
				schemaResult.TypeRead = utils.WrapNullableTypeEncoder(schemaResult.TypeRead)
				schemaResult.TypeWrite = utils.WrapNullableTypeEncoder(schemaResult.TypeWrite)
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

		if isEmptyObject {
			result = transformNullableObjectPropertiesSchema(oc.state.schema, result, nullable, fieldPaths)
		}

		return result, nil
	}

	unionSchemas := []SchemaInfoCache{}
	oneOfInfos := []SchemaInfoCache{}

	for i, item := range proxies {
		schemaResult, err := newOASSchemaBuilder(oc.state, oc.apiPath, oc.location).
			getSchemaTypeFromProxy(item, nullable, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, err
		}

		unionSchemas = append(unionSchemas, *schemaResult)
		if unionType == oasOneOf && schemaResult != nil {
			oneOfInfos = append(oneOfInfos, *schemaResult)
		}
	}

	result := mergeUnionTypeSchemas(oc.state.schema, baseSchema, unionSchemas, unionType, fieldPaths)
	if isEmptyObject {
		result = transformNullableObjectPropertiesSchema(oc.state.schema, result, nullable, fieldPaths)
	}

	result.OneOf = oneOfInfos

	return result, nil
}
