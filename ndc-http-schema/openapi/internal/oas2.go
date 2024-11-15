package internal

import (
	"fmt"
	"log/slog"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	"github.com/pb33f/libopenapi/orderedmap"
)

// OAS2Builder the NDC schema builder from OpenAPI 2.0 specification
type OAS2Builder struct {
	*ConvertOptions

	schema *rest.NDCHttpSchema
	// stores prebuilt and evaluating information of component schema types.
	// some undefined schema types aren't stored in either object nor scalar,
	// or self-reference types that haven't added into the object_types map yet.
	// This cache temporarily stores them to avoid infinite recursive reference.
	schemaCache map[string]SchemaInfoCache
}

// NewOAS2Builder creates an OAS3Builder instance
func NewOAS2Builder(schema *rest.NDCHttpSchema, options ConvertOptions) *OAS2Builder {
	builder := &OAS2Builder{
		schema:         schema,
		schemaCache:    make(map[string]SchemaInfoCache),
		ConvertOptions: applyConvertOptions(options),
	}

	return builder
}

// Schema returns the inner NDC HTTP schema
func (oc *OAS2Builder) Schema() *rest.NDCHttpSchema {
	return oc.schema
}

func (oc *OAS2Builder) BuildDocumentModel(docModel *libopenapi.DocumentModel[v2.Swagger]) error {
	if docModel.Model.Info != nil {
		oc.schema.Settings.Version = docModel.Model.Info.Version
	}

	if docModel.Model.Host != "" {
		scheme := "https"
		for _, s := range docModel.Model.Schemes {
			if strings.HasPrefix(s, "http") {
				scheme = s
				break
			}
		}
		envName := utils.StringSliceToConstantCase([]string{oc.EnvPrefix, "SERVER_URL"})
		serverURL := fmt.Sprintf("%s://%s%s", scheme, docModel.Model.Host, docModel.Model.BasePath)
		oc.schema.Settings.Servers = append(oc.schema.Settings.Servers, rest.ServerConfig{
			URL: sdkUtils.NewEnvString(envName, serverURL),
		})
	}

	for iterPath := docModel.Model.Paths.PathItems.First(); iterPath != nil; iterPath = iterPath.Next() {
		if err := oc.pathToNDCOperations(iterPath); err != nil {
			return err
		}
	}

	if docModel.Model.Definitions != nil {
		for cSchema := docModel.Model.Definitions.Definitions.First(); cSchema != nil; cSchema = cSchema.Next() {
			if err := oc.convertComponentSchemas(cSchema); err != nil {
				return err
			}
		}
	}

	if docModel.Model.SecurityDefinitions != nil && docModel.Model.SecurityDefinitions.Definitions != nil {
		oc.schema.Settings.SecuritySchemes = make(map[string]rest.SecurityScheme)
		for scheme := docModel.Model.SecurityDefinitions.Definitions.First(); scheme != nil; scheme = scheme.Next() {
			err := oc.convertSecuritySchemes(scheme)
			if err != nil {
				return err
			}
		}
	}

	oc.schema.Settings.Security = convertSecurities(docModel.Model.Security)
	if err := cleanUnusedSchemaTypes(oc.schema); err != nil {
		return err
	}

	return nil
}

func (oc *OAS2Builder) convertSecuritySchemes(scheme orderedmap.Pair[string, *v2.SecurityScheme]) error {
	key := scheme.Key()
	security := scheme.Value()
	if security == nil {
		return nil
	}
	result := rest.SecurityScheme{}
	switch security.Type {
	case "apiKey":
		result.Type = rest.APIKeyScheme
		inLocation, err := rest.ParseAPIKeyLocation(security.In)
		if err != nil {
			return err
		}
		apiConfig := rest.APIKeyAuthConfig{
			In:   inLocation,
			Name: security.Name,
		}
		valueEnv := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key}))
		result.Value = &valueEnv
		result.APIKeyAuthConfig = &apiConfig
	case "basic":
		result.Type = rest.HTTPAuthScheme
		httpConfig := rest.HTTPAuthConfig{
			Scheme: "Basic",
			Header: "Authorization",
		}
		valueEnv := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "TOKEN"}))
		result.Value = &valueEnv
		result.HTTPAuthConfig = &httpConfig
	case "oauth2":
		var flowType rest.OAuthFlowType
		switch security.Flow {
		case "accessCode":
			flowType = rest.AuthorizationCodeFlow
		case "implicit":
			flowType = rest.ImplicitFlow
		case "password":
			flowType = rest.PasswordFlow
		case "application":
			flowType = rest.ClientCredentialsFlow
		}
		flow := rest.OAuthFlow{
			AuthorizationURL: security.AuthorizationUrl,
			TokenURL:         security.TokenUrl,
		}

		if security.Scopes != nil {
			scopes := make(map[string]string)
			for scope := security.Scopes.Values.First(); scope != nil; scope = scope.Next() {
				scopes[scope.Key()] = scope.Value()
			}
			flow.Scopes = scopes
		}
		result.Type = rest.OAuth2Scheme
		result.OAuth2Config = &rest.OAuth2Config{
			Flows: map[rest.OAuthFlowType]rest.OAuthFlow{
				flowType: flow,
			},
		}
	default:
		return fmt.Errorf("invalid security scheme: %s", security.Type)
	}

	oc.schema.Settings.SecuritySchemes[key] = result
	return nil
}

func (oc *OAS2Builder) pathToNDCOperations(pathItem orderedmap.Pair[string, *v2.PathItem]) error {
	pathKey := pathItem.Key()
	pathValue := pathItem.Value()

	funcGet, funcName, err := newOAS2OperationBuilder(oc).BuildFunction(pathKey, pathValue.Get, pathValue.Parameters)
	if err != nil {
		return err
	}
	if funcGet != nil {
		oc.schema.Functions[funcName] = *funcGet
	}

	procPost, procPostName, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "post", pathValue.Post, pathValue.Parameters)
	if err != nil {
		return err
	}
	if procPost != nil {
		oc.schema.Procedures[procPostName] = *procPost
	}

	procPut, procPutName, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "put", pathValue.Put, pathValue.Parameters)
	if err != nil {
		return err
	}
	if procPut != nil {
		oc.schema.Procedures[procPutName] = *procPut
	}

	procPatch, procPatchName, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "patch", pathValue.Patch, pathValue.Parameters)
	if err != nil {
		return err
	}
	if procPatch != nil {
		oc.schema.Procedures[procPatchName] = *procPatch
	}

	procDelete, procDeleteName, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "delete", pathValue.Delete, pathValue.Parameters)
	if err != nil {
		return err
	}
	if procDelete != nil {
		oc.schema.Procedures[procDeleteName] = *procDelete
	}
	return nil
}

func (oc *OAS2Builder) convertComponentSchemas(schemaItem orderedmap.Pair[string, *base.SchemaProxy]) error {
	typeKey := schemaItem.Key()
	typeValue := schemaItem.Value()
	typeSchema := typeValue.Schema()

	oc.Logger.Debug("component schema", slog.String("name", typeKey))
	if typeSchema == nil {
		return nil
	}
	typeEncoder, schemaResult, err := newOAS2SchemaBuilder(oc, "", rest.InBody).getSchemaType(typeSchema, []string{typeKey})

	var typeName string
	if typeEncoder != nil {
		typeName = getNamedType(typeEncoder, true, "")
	}
	cacheKey := "#/definitions/" + typeKey
	// treat no-property objects as a Arbitrary JSON scalar
	if typeEncoder == nil || typeName == string(rest.ScalarJSON) {
		refName := utils.ToPascalCase(typeKey)
		scalar := schema.NewScalarType()
		scalar.Representation = schema.NewTypeRepresentationJSON().Encode()
		oc.schema.ScalarTypes[refName] = *scalar
		oc.schemaCache[cacheKey] = SchemaInfoCache{
			Name:       refName,
			Schema:     schema.NewNamedType(refName),
			TypeSchema: schemaResult,
		}
	} else {
		oc.schemaCache[cacheKey] = SchemaInfoCache{
			Name:       typeName,
			Schema:     typeEncoder,
			TypeSchema: schemaResult,
		}
	}

	return err
}

// build a named type for JSON scalar
func (oc *OAS2Builder) buildScalarJSON() *schema.NamedType {
	scalarName := string(rest.ScalarJSON)
	if _, ok := oc.schema.ScalarTypes[scalarName]; !ok {
		oc.schema.ScalarTypes[scalarName] = *defaultScalarTypes[rest.ScalarJSON]
	}
	return schema.NewNamedType(scalarName)
}
