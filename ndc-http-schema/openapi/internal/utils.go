package internal

import (
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"unicode"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"gopkg.in/yaml.v3"
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

func getScalarFromType(sm *rest.NDCHttpSchema, names []string, format string, enumNodes []*yaml.Node, apiPath string, fieldPaths []string) string {
	var scalarName string
	var scalarType *schema.ScalarType

	if len(names) != 1 {
		scalarName = "JSON"
		scalarType = defaultScalarTypes[rest.ScalarJSON]
	} else {
		scalarName, scalarType = getScalarFromNamedType(sm, names, format, enumNodes, apiPath, fieldPaths)
	}

	if _, ok := sm.ScalarTypes[scalarName]; !ok {
		sm.ScalarTypes[scalarName] = *scalarType
	}

	return scalarName
}

func getScalarFromNamedType(sm *rest.NDCHttpSchema, names []string, format string, enumNodes []*yaml.Node, apiPath string, fieldPaths []string) (string, *schema.ScalarType) {
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
		schemaEnumLength := len(enumNodes)
		if schemaEnumLength > 0 {
			enums := make([]string, schemaEnumLength)
			for i, enum := range enumNodes {
				enums[i] = enum.Value
			}
			scalarType = schema.NewScalarType()
			scalarType.Representation = schema.NewTypeRepresentationEnum(enums).Encode()

			// build scalar name strategies
			// 1. combine resource name and field name
			apiPath = strings.TrimPrefix(apiPath, "/")
			if apiPath != "" {
				apiPaths := strings.Split(apiPath, "/")
				resourceName := fieldPaths[0]
				if len(apiPaths) > 0 {
					resourceName = apiPaths[0]
				}
				enumName := "Enum"
				if len(fieldPaths) > 1 {
					enumName = fieldPaths[len(fieldPaths)-1]
				}

				scalarName = utils.StringSliceToPascalCase([]string{resourceName, enumName})
				if canSetEnumToSchema(sm, scalarName, enums) {
					return scalarName, scalarType
				}
			}

			// 2. if the scalar type exists, fallback to field paths
			scalarName = utils.StringSliceToPascalCase(fieldPaths)
			if canSetEnumToSchema(sm, scalarName, enums) {
				return scalarName, scalarType
			}

			// 3. Reuse above name with Enum suffix
			scalarName += "Enum"

			return scalarName, scalarType
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

// getMethodAlias merge method alias map with default value
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

// check if the OAS type is a scalar
func isPrimitiveScalar(names []string) bool {
	for _, name := range names {
		if !slices.Contains([]string{"boolean", "integer", "number", "string", "file", "long", "null"}, name) {
			return false
		}
	}

	return true
}

// get the inner named type of the type encoder
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

func isNullableType(input schema.TypeEncoder) bool {
	_, ok := input.(*schema.NullableType)

	return ok
}

// Find common fields in all objects to merge the type.
// If they have the same type, we don't need to wrap it with the nullable type.
func mergeUnionObjects(httpSchema *rest.NDCHttpSchema, dest *rest.ObjectType, srcObjects []rest.ObjectType, unionType oasUnionType, fieldPaths []string) error {
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

			var ok bool
			unionField, ok = mergeUnionTypes(httpSchema, field.Type, unionField.Type, append(fieldPaths, key))
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

	return nil
}

func mergeUnionTypes(httpSchema *rest.NDCHttpSchema, a schema.Type, b schema.Type, fieldPaths []string) (rest.ObjectField, bool) {
	bn, bNullErr := b.AsNullable()
	bType := b
	if bNullErr == nil {
		bType = bn.UnderlyingType
	}

	var result rest.ObjectField
	var isEqual bool

	switch at := a.Interface().(type) {
	case *schema.NullableType:
		result, ok := mergeUnionTypes(httpSchema, at.UnderlyingType, bType, fieldPaths)
		if !ok {
			return rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
				},
			}, false
		}

		if !isNullableType(result.Type.Interface()) {
			result.Type = (schema.NullableType{
				Type:           schema.TypeNullable,
				UnderlyingType: result.Type,
			}).Encode()
		}

		return result, true
	case *schema.ArrayType:
		bt, err := bType.AsArray()
		if err != nil {
			break
		}

		result, isEqual = mergeUnionTypes(httpSchema, at.ElementType, bt.ElementType, fieldPaths)
		if !isEqual {
			result = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: schema.NewArrayType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
				},
			}
		} else {
			result.Type = schema.ArrayType{
				Type:        schema.TypeArray,
				ElementType: result.Type,
			}.Encode()
		}
	case *schema.NamedType:
		bt, err := bType.AsNamed()
		if err != nil {
			break
		}

		if at.Name == bt.Name {
			result = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: a,
				},
			}

			isEqual = true

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

			result = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: schema.NewNamedType(newName).Encode(),
				},
			}
			isEqual = true

			break
		}

		scalarName := rest.ScalarJSON
		switch {
		case typeRepA == "" || typeRepB == "" || typeRepA == schema.TypeRepresentationTypeJSON || typeRepB == schema.TypeRepresentationTypeJSON:
		case typeRepA == typeRepB:
			sn, ok := typeRepresentationToScalarNameRelationship[typeRepA]
			if ok {
				scalarName = sn
			}
		case slices.Contains(integerTypeRepresentations, typeRepA) && slices.Contains(integerTypeRepresentations, typeRepB):
			if typeRepA == schema.TypeRepresentationTypeInt64 || typeRepB == schema.TypeRepresentationTypeInt64 {
				scalarName = rest.ScalarInt64
			} else {
				scalarName = rest.ScalarInt32
			}
		case slices.Contains(floatTypeRepresentations, typeRepA) && slices.Contains(floatTypeRepresentations, typeRepB):
			scalarName = rest.ScalarFloat64
		case slices.Contains(stringTypeRepresentations, typeRepA) && slices.Contains(stringTypeRepresentations, typeRepB):
			scalarName = rest.ScalarString
		}

		result = rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: schema.NewNamedType(string(scalarName)).Encode(),
			},
		}
	}

	if len(result.ObjectField.Type) == 0 {
		result = rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: schema.NewNamedType(string(rest.ScalarJSON)).Encode(),
			},
		}
	}

	if bNullErr != nil && !isNullableType(result.Type.Interface()) {
		result.Type = (schema.NullableType{
			Type:           schema.TypeNullable,
			UnderlyingType: result.Type,
		}).Encode()
	}

	return result, isEqual
}

// encodeHeaderArgumentName encodes header key to NDC schema field name
func encodeHeaderArgumentName(name string) string {
	return "header" + utils.ToPascalCase(name)
}

// evaluate and filter invalid types in allOf, anyOf or oneOf schemas
func evalSchemaProxiesSlice(schemaProxies []*base.SchemaProxy, location rest.ParameterLocation) ([]*base.SchemaProxy, *base.Schema, bool) {
	var results []*base.SchemaProxy
	var typeNames []string
	nullable := false
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
		if location == rest.InQuery && (sc.Type[0] == "string" && len(sc.Enum) == 1 && (sc.Enum[0] == nil || sc.Enum[0].Value == "")) {
			nullable = true

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
		}, nullable
	}

	return results, nil, nullable
}

func formatWriteObjectName(name string) string {
	return name + "Input"
}

func errParameterSchemaEmpty(fieldPaths []string) error {
	return fmt.Errorf("parameter schema of $.%s is empty", strings.Join(fieldPaths, "."))
}

// redirection and information response status codes aren't supported
func isUnsupportedResponseCodes[T int | int64](code T) bool {
	return code < 200 || (code >= 300 && code < 400)
}

// format the operation name and remove special characters
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

// check if the XML object doesn't have any child element.
func isXMLLeafObject(objectType rest.ObjectType) bool {
	for _, field := range objectType.Fields {
		if field.HTTP == nil || field.HTTP.XML == nil || !field.HTTP.XML.Attribute {
			return false
		}
	}

	return true
}

func createTLSConfig(keys []string) *rest.TLSConfig {
	caPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CA_PEM")))
	caFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CA_FILE")))
	certPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CERT_PEM")))
	certFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "CERT_FILE")))
	keyPem := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "KEY_PEM")))
	keyFile := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "KEY_FILE")))
	serverName := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase(append(keys, "SERVER_NAME")))
	insecureSkipVerify := sdkUtils.NewEnvBool(utils.StringSliceToConstantCase(append(keys, "INSECURE_SKIP_VERIFY")), false)
	includeSystemCACertsPool := sdkUtils.NewEnvBool(utils.StringSliceToConstantCase(append(keys, "INCLUDE_SYSTEM_CA_CERTS_POOL")), false)

	return &rest.TLSConfig{
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

func evalOperationPath(httpSchema *rest.NDCHttpSchema, rawPath string, arguments map[string]rest.ArgumentInfo) (string, map[string]rest.ArgumentInfo, error) {
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

		httpSchema.AddScalar(string(rest.ScalarString), *defaultScalarTypes[rest.ScalarString])
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
