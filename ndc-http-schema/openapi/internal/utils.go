package internal

import (
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/hasura/ndc-http/exhttp"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
	"github.com/pb33f/libopenapi/datamodel/high/base"
)

func applyConvertOptions(opts ConvertOptions) *ConvertOptions {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	opts.MethodAlias = getMethodAlias(opts.MethodAlias)

	return &opts
}

func buildPathMethodName(apiPath string, method string, options *ConvertOptions) string {
	if options.TrimPrefix != "" {
		apiPath = strings.TrimPrefix(apiPath, options.TrimPrefix)
	}
	encodedPath := utils.ToPascalCase(bracketRegexp.ReplaceAllString(strings.TrimLeft(apiPath, "/"), ""))
	if alias, ok := options.MethodAlias[method]; ok {
		method = alias
	}

	return utils.ToCamelCase(method + encodedPath)
}

// getMethodAlias merge method alias map with default value.
func getMethodAlias(inputs ...map[string]string) map[string]string {
	methodAlias := map[string]string{
		"get":    "get",
		"post":   "post",
		"put":    "put",
		"patch":  "patch",
		"delete": "delete",
	}
	for _, input := range inputs {
		for k, alias := range input {
			methodAlias[k] = alias
		}
	}

	return methodAlias
}

func convertSecurities(securities []*base.SecurityRequirement) rest.AuthSecurities {
	var results rest.AuthSecurities
	for _, security := range securities {
		s := convertSecurity(security)
		if s != nil {
			results = append(results, s)
		}
	}

	return results
}

func convertSecurity(security *base.SecurityRequirement) rest.AuthSecurity {
	if security == nil {
		return nil
	}
	results := make(map[string][]string)
	for s := security.Requirements.First(); s != nil; s = s.Next() {
		v := s.Value()
		if v == nil {
			v = []string{}
		}
		results[s.Key()] = v
	}

	return results
}

// get the inner named type of the type encoder.
func getNamedType(typeSchema schema.TypeEncoder, recursive bool, defaultValue string) string {
	switch ty := typeSchema.(type) {
	case *schema.NullableType:
		return getNamedType(ty.UnderlyingType.Interface(), recursive, defaultValue)
	case *schema.ArrayType:
		if !recursive {
			return defaultValue
		}

		return getNamedType(ty.ElementType.Interface(), recursive, defaultValue)
	case *schema.NamedType:
		return ty.Name
	default:
		return defaultValue
	}
}

func unwrapNullableUnionTypeSchemas(inputs []SchemaInfoCache) ([]SchemaInfoCache, bool, bool) {
	var readNullable bool
	var writeNullable bool
	results := make([]SchemaInfoCache, len(inputs))
	for i, item := range inputs {
		typeRead, rn, _ := utils.UnwrapNullableTypeEncoder(item.TypeRead)
		readNullable = readNullable || rn
		item.TypeRead = typeRead

		typeWrite, wn, _ := utils.UnwrapNullableTypeEncoder(item.TypeWrite)
		writeNullable = writeNullable || wn
		item.TypeWrite = typeWrite

		results[i] = item
	}

	return results, readNullable, writeNullable
}

func mergeUnionTypeSchemas(httpSchema *rest.NDCHttpSchema, baseSchema *base.Schema, inputs []SchemaInfoCache, unionType oasUnionType, fieldPaths []string) *SchemaInfoCache {
	result, ok := mergeUnionTypeSchemasRecursive(httpSchema, baseSchema, inputs, unionType, fieldPaths)
	if ok {
		return result
	}

	scalarName := rest.ScalarJSON
	scalarType := schema.NewNamedType(string(scalarName))
	typeSchema := createSchemaFromOpenAPISchema(baseSchema)

	if baseSchema.Description != "" {
		typeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
	}

	return &SchemaInfoCache{
		TypeRead:   scalarType,
		TypeWrite:  scalarType,
		TypeSchema: typeSchema,
	}
}

func mergeUnionTypeSchemasRecursive(httpSchema *rest.NDCHttpSchema, baseSchema *base.Schema, inputs []SchemaInfoCache, unionType oasUnionType, fieldPaths []string) (*SchemaInfoCache, bool) {
	newInputs, readNullable, writeNullable := unwrapNullableUnionTypeSchemas(inputs)
	var result *SchemaInfoCache
	var ok bool

	switch tr := inputs[0].TypeRead.(type) {
	case *schema.NullableType:
		result, ok = mergeUnionTypeSchemasRecursive(httpSchema, baseSchema, newInputs, unionType, fieldPaths)
	case *schema.ArrayType:
		elemInputs := make([]SchemaInfoCache, len(inputs))
		for i, item := range newInputs {
			arrRead, isArray := item.TypeRead.(*schema.ArrayType)
			if !isArray {
				return nil, false
			}
			arrWrite, isArray := item.TypeWrite.(*schema.ArrayType)
			if !isArray {
				return nil, false
			}

			item.TypeRead = arrRead.ElementType.Interface()
			item.TypeWrite = arrWrite.ElementType.Interface()
			elemInputs[i] = item
		}

		result, ok = mergeUnionTypeSchemasRecursive(httpSchema, baseSchema, elemInputs, unionType, fieldPaths)
		if !ok {
			return nil, false
		}

		result.TypeRead = schema.NewArrayType(result.TypeRead)
		result.TypeWrite = schema.NewArrayType(result.TypeWrite)
	case *schema.NamedType:
		result = &SchemaInfoCache{
			TypeSchema: &rest.TypeSchema{},
		}
		if _, isScalar := httpSchema.ScalarTypes[tr.Name]; isScalar {
			for i, item := range newInputs {
				if i == 0 {
					result.TypeRead = item.TypeRead
					result.TypeWrite = item.TypeWrite

					continue
				}

				rt, isMatched := mergeUnionTypes(httpSchema, result.TypeRead.Encode(), item.TypeRead.Encode(), fieldPaths)
				if !isMatched {
					return nil, false
				}

				wt, isMatched := mergeUnionTypes(httpSchema, result.TypeWrite.Encode(), item.TypeWrite.Encode(), fieldPaths)
				if !isMatched {
					return nil, false
				}

				result.TypeRead = rt
				result.TypeWrite = wt
			}
			ok = true

			break
		}

		_, isObject := httpSchema.ObjectTypes[tr.Name]
		if !isObject {
			return nil, false
		}

		readObjects := make([]rest.ObjectType, len(newInputs))
		writeObjects := make([]rest.ObjectType, len(newInputs))
		for i, item := range newInputs {
			rNamed, isNamedType := item.TypeRead.(*schema.NamedType)
			if !isNamedType {
				return nil, false
			}
			ro, isObject := httpSchema.ObjectTypes[rNamed.Name]
			if !isObject {
				return nil, false
			}
			readObjects[i] = ro

			wNamed, isNamedType := item.TypeWrite.(*schema.NamedType)
			if !isNamedType {
				return nil, false
			}
			wo, isObject := httpSchema.ObjectTypes[wNamed.Name]
			if !isObject {
				return nil, false
			}
			writeObjects[i] = wo
		}

		readObject := rest.ObjectType{
			Fields: map[string]rest.ObjectField{},
		}
		writeObject := rest.ObjectType{
			Fields: map[string]rest.ObjectField{},
		}

		if baseSchema.Description != "" {
			description := utils.StripHTMLTags(baseSchema.Description)
			readObject.Description = &description
			writeObject.Description = &description
		}

		mergeUnionObjects(httpSchema, &readObject, readObjects, unionType, fieldPaths)
		mergeUnionObjects(httpSchema, &writeObject, writeObjects, unionType, fieldPaths)

		refName := utils.ToPascalCase(strings.Join(fieldPaths, " "))
		writeRefName := formatWriteObjectName(refName)
		if len(readObject.Fields) > 0 {
			httpSchema.ObjectTypes[refName] = readObject
		}
		if len(writeObject.Fields) > 0 {
			httpSchema.ObjectTypes[writeRefName] = writeObject
		}

		typeSchema := &rest.TypeSchema{
			Type: []string{"object"},
		}

		result.TypeRead = schema.NewNamedType(refName)
		result.TypeWrite = schema.NewNamedType(writeRefName)
		result.TypeSchema = typeSchema
		ok = true
	default:
		return nil, false
	}

	if !ok {
		return nil, false
	}

	if readNullable {
		result.TypeRead = utils.WrapNullableTypeEncoder(result.TypeRead)
	}

	if writeNullable {
		result.TypeWrite = utils.WrapNullableTypeEncoder(result.TypeWrite)
	}

	return result, ok
}

func mergeUnionTypes(httpSchema *rest.NDCHttpSchema, a schema.Type, b schema.Type, fieldPaths []string) (schema.TypeEncoder, bool) {
	bn, bNullErr := b.AsNullable()
	bType := b
	if bNullErr == nil {
		bType = bn.UnderlyingType
	}

	var result schema.TypeEncoder
	var isMatched bool

	switch at := a.Interface().(type) {
	case *schema.NullableType:
		result, ok := mergeUnionTypes(httpSchema, at.UnderlyingType, bType, fieldPaths)

		return utils.WrapNullableTypeEncoder(result), ok
	case *schema.ArrayType:
		bt, err := bType.AsArray()
		if err != nil {
			break
		}

		result, isMatched = mergeUnionTypes(httpSchema, at.ElementType, bt.ElementType, fieldPaths)
		result = schema.NewArrayType(result)
	case *schema.NamedType:
		bt, err := bType.AsNamed()
		if err != nil {
			break
		}

		if at.Name == bt.Name {
			result = at
			isMatched = true

			break
		}

		// if both types are enum scalars, a new enum scalar is created with the merged value set of both enums.
		var typeRepA, typeRepB schema.TypeRepresentationType
		var enumA, enumB *schema.TypeRepresentationEnum
		scalarA, ok := httpSchema.ScalarTypes[at.Name]
		if ok {
			typeRepA, _ = scalarA.Representation.Type()
			enumA, _ = scalarA.Representation.AsEnum()
		}

		scalarB, ok := httpSchema.ScalarTypes[bt.Name]
		if ok {
			typeRepB, _ = scalarB.Representation.Type()
			enumB, _ = scalarB.Representation.AsEnum()
		}

		if enumA != nil && enumB != nil {
			enumValues := utils.SliceUnique(append(enumA.OneOf, enumB.OneOf...))
			newScalar := schema.NewScalarType()
			newScalar.Representation = schema.NewTypeRepresentationEnum(enumValues).Encode()

			newName := utils.StringSliceToPascalCase(append(fieldPaths, "Enum"))
			httpSchema.ScalarTypes[newName] = *newScalar

			result = schema.NewNamedType(newName)
			isMatched = true

			break
		}

		scalarName := rest.ScalarJSON
		switch {
		case typeRepA == "" || typeRepB == "" || typeRepA == schema.TypeRepresentationTypeJSON || typeRepB == schema.TypeRepresentationTypeJSON:
		case typeRepA == typeRepB:
			sn, ok := typeRepresentationToScalarNameRelationship[typeRepA]
			if ok {
				scalarName = sn
				isMatched = true
			}
		case slices.Contains(integerTypeRepresentations, typeRepA) && slices.Contains(integerTypeRepresentations, typeRepB):
			if typeRepA == schema.TypeRepresentationTypeInt64 || typeRepB == schema.TypeRepresentationTypeInt64 {
				scalarName = rest.ScalarInt64
			} else {
				scalarName = rest.ScalarInt32
			}
			isMatched = true
		case slices.Contains(floatTypeRepresentations, typeRepA) && slices.Contains(floatTypeRepresentations, typeRepB):
			scalarName = rest.ScalarFloat64
			isMatched = true
		// use boolean if the union type if oneOf boolean or enum (true, false)
		case (enumA != nil && len(enumA.OneOf) == 2 && slices.Contains(enumA.OneOf, "true") && slices.Contains(enumA.OneOf, "false") && typeRepB == schema.TypeRepresentationTypeBoolean) ||
			(enumB != nil && len(enumB.OneOf) == 2 && slices.Contains(enumB.OneOf, "true") && slices.Contains(enumB.OneOf, "false") && typeRepA == schema.TypeRepresentationTypeBoolean):
			scalarName = rest.ScalarBoolean
			isMatched = true
		case slices.Contains(stringTypeRepresentations, typeRepA) && slices.Contains(stringTypeRepresentations, typeRepB):
			scalarName = rest.ScalarString
			isMatched = true
		}

		result = schema.NewNamedType(string(scalarName))
	}

	if result == nil {
		result = schema.NewNamedType(string(rest.ScalarJSON))
	}

	if bNullErr != nil {
		result = utils.WrapNullableTypeEncoder(result)
	}

	return result, isMatched
}

// encodeHeaderArgumentName encodes header key to NDC schema field name.
func encodeHeaderArgumentName(name string) string {
	return "header" + utils.ToPascalCase(name)
}

func formatWriteObjectName(name string) string {
	return name + "Input"
}

func errParameterSchemaEmpty(fieldPaths []string) error {
	return fmt.Errorf("parameter schema of $.%s is empty", strings.Join(fieldPaths, "."))
}

// redirection and information response status codes aren't supported.
func isUnsupportedResponseCodes[T int | int64](code T) bool {
	return code < 200 || (code >= 300 && code < 400)
}

// format the operation name and remove special characters.
func formatOperationName(input string) string {
	if input == "" {
		return ""
	}

	sb := strings.Builder{}
	for i, c := range input {
		if unicode.IsLetter(c) {
			sb.WriteRune(c)

			continue
		}

		if unicode.IsNumber(c) && i > 0 {
			sb.WriteRune(c)

			continue
		}

		sb.WriteRune('_')
	}

	return sb.String()
}

func buildUniqueOperationName(httpSchema *rest.NDCHttpSchema, operationId, pathKey, method string, options *ConvertOptions) string {
	opName := formatOperationName(operationId)
	exists := opName == ""
	if !exists {
		_, exists = httpSchema.Functions[opName]
		if !exists {
			_, exists = httpSchema.Procedures[opName]
		}
	}

	if exists {
		opName = buildPathMethodName(pathKey, method, options)
	}

	return opName
}

// check if the XML object doesn't have any child element.
func isXMLLeafObject(objectType rest.ObjectType) bool {
	for _, field := range objectType.Fields {
		if field.HTTP == nil || field.HTTP.XML == nil || !field.HTTP.XML.Attribute {
			return false
		}
	}

	return true
}

func createTLSConfig(keys []string) *exhttp.TLSConfig {
	caPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CA_PEM")))
	caFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CA_FILE")))
	certPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CERT_PEM")))
	certFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CERT_FILE")))
	keyPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "KEY_PEM")))
	keyFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "KEY_FILE")))
	serverName := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "SERVER_NAME")))
	insecureSkipVerify := sdkUtils.NewEnvBool(utils.StringSliceToConstantCase(append(keys, "INSECURE_SKIP_VERIFY")), false)
	includeSystemCACertsPool := sdkUtils.NewEnvBool(utils.StringSliceToConstantCase(append(keys, "INCLUDE_SYSTEM_CA_CERTS_POOL")), false)

	return &exhttp.TLSConfig{
		CAFile:                   &caFile,
		CAPem:                    &caPem,
		CertFile:                 &certFile,
		CertPem:                  &certPem,
		KeyFile:                  &keyFile,
		KeyPem:                   &keyPem,
		InsecureSkipVerify:       &insecureSkipVerify,
		IncludeSystemCACertsPool: &includeSystemCACertsPool,
		ServerName:               &serverName,
	}
}

func evalOperationPath(rawPath string, arguments map[string]rest.ArgumentInfo) (string, map[string]rest.ArgumentInfo, error) {
	var pathURL *url.URL
	var isAbsolute bool
	var err error

	if strings.HasPrefix(rawPath, "http") {
		isAbsolute = true
		pathURL, err = url.Parse(rawPath)
		if err != nil {
			return "", nil, err
		}
	} else {
		pathURL, err = url.Parse("http://example.local" + rawPath)
		if err != nil {
			return "", nil, err
		}
	}

	newQuery := url.Values{}
	q := pathURL.Query()
	for key, value := range q {
		if len(value) == 0 || value[0] == "" {
			continue
		}

		matches := oasVariableRegex.FindStringSubmatch(value[0])
		if len(matches) < 2 {
			newQuery.Set(key, value[0])

			continue
		}

		variableName := matches[1]
		if _, ok := arguments[variableName]; ok {
			// the argument exists, skip the next value
			continue
		}

		arguments[variableName] = rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: schema.NewNamedType(string(rest.ScalarString)).Encode(),
			},
			HTTP: &rest.RequestParameter{
				Name: variableName,
				In:   rest.InQuery,
				Schema: &rest.TypeSchema{
					Type: []string{"string"},
				},
			},
		}
	}

	pathURL.RawQuery = newQuery.Encode()
	if isAbsolute {
		return pathURL.String(), arguments, nil
	}

	queryString := pathURL.Query().Encode()

	if queryString != "" {
		queryString = "?" + queryString
	}

	fragment := pathURL.EscapedFragment()
	if fragment != "" {
		fragment = "#" + fragment
	}

	return pathURL.Path + queryString + fragment, arguments, nil
}

func transformNullableObjectProperties(httpSchema *rest.NDCHttpSchema, input schema.Type, newName string) (schema.TypeEncoder, bool) {
	switch t := input.Interface().(type) {
	case *schema.NullableType:
		result, isObject := transformNullableObjectProperties(httpSchema, t.UnderlyingType, newName)

		return utils.WrapNullableTypeEncoder(result), isObject
	case *schema.ArrayType:
		result, isObject := transformNullableObjectProperties(httpSchema, t.ElementType, newName)

		return schema.NewArrayType(result), isObject
	case *schema.NamedType:
		if _, ok := httpSchema.ScalarTypes[t.Name]; ok {
			return t, false
		}

		objType, ok := httpSchema.ObjectTypes[t.Name]
		if !ok {
			return t, false
		}

		newObjectType := rest.ObjectType{
			Description: objType.Description,
			Fields:      make(map[string]rest.ObjectField),
			XML:         objType.XML,
		}

		for key, field := range objType.Fields {
			fieldType := field.Type.Interface()
			field.Type = utils.WrapNullableTypeEncoder(fieldType).Encode()
			newObjectType.Fields[key] = field
		}

		httpSchema.ObjectTypes[newName] = newObjectType

		return schema.NewNamedType(newName), true
	default:
		return t, false
	}
}

func transformNullableObjectPropertiesSchema(httpSchema *rest.NDCHttpSchema, result *SchemaInfoCache, nullable bool, fieldPaths []string) *SchemaInfoCache {
	readSchemaName := utils.StringSliceToPascalCase(fieldPaths)
	writeSchemaName := formatWriteObjectName(readSchemaName)

	var ok bool
	result.TypeRead, ok = transformNullableObjectProperties(httpSchema, result.TypeRead.Encode(), readSchemaName)
	if !ok {
		return createSchemaInfoJSONScalar(nullable)
	}

	result.TypeWrite, ok = transformNullableObjectProperties(httpSchema, result.TypeWrite.Encode(), writeSchemaName)
	if !ok {
		return createSchemaInfoJSONScalar(nullable)
	}

	return result
}

func createSchemaInfoJSONScalar(nullable bool) *SchemaInfoCache {
	scalarName := rest.ScalarJSON
	var result schema.TypeEncoder = schema.NewNamedType(string(scalarName))
	if nullable {
		result = schema.NewNullableType(result)
	}

	return &SchemaInfoCache{
		TypeRead:   result,
		TypeWrite:  result,
		TypeSchema: &rest.TypeSchema{},
	}
}
