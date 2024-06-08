package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/hasura/ndc-rest-schema/command"
	rest "github.com/hasura/ndc-rest-schema/schema"
	restUtils "github.com/hasura/ndc-rest-schema/utils"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// GetSchema gets the connector's schema.
func (c *RESTConnector) GetSchema(ctx context.Context, configuration *Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// build NDC REST schema from file list
func buildSchemaFiles(configDir string, files []ConfigItem, logger *slog.Logger) ([]ndcRestSchemaWithName, map[string][]string) {
	schemas := make([]ndcRestSchemaWithName, len(files))
	errors := make(map[string][]string)
	for i, file := range files {
		var errs []string
		schemaOutput, err := buildSchemaFile(configDir, &file, logger)
		if err != nil {
			errs = append(errs, err.Error())
		}
		if schemaOutput != nil {
			schemas[i] = ndcRestSchemaWithName{
				name:   file.File,
				schema: schemaOutput,
			}
		}
		if len(errs) > 0 {
			errors[file.File] = errs
		}
	}

	return schemas, errors
}

func buildSchemaFile(configDir string, conf *ConfigItem, logger *slog.Logger) (*rest.NDCRestSchema, error) {
	if conf.ConvertConfig.File == "" {
		return nil, errors.New("file path is empty")
	}
	command.ResolveConvertConfigArguments(&conf.ConvertConfig, configDir, nil)
	ndcSchema, err := command.ConvertToNDCSchema(&conf.ConvertConfig, logger)
	if err != nil {
		return nil, err
	}

	buildRESTArguments(ndcSchema, conf)

	return ndcSchema, nil
}

func (c *RESTConnector) applyNDCRestSchemas(schemas []ndcRestSchemaWithName) map[string][]string {
	ndcSchema := &schema.SchemaResponse{
		Collections: []schema.CollectionInfo{},
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
		ObjectTypes: make(schema.SchemaResponseObjectTypes),
	}
	errors := make(map[string][]string)

	for _, item := range schemas {
		settings := item.schema.Settings
		if settings == nil {
			settings = &rest.NDCRestSettings{}
		}
		meta := RESTMetadata{
			settings:   settings,
			functions:  map[string]rest.RESTFunctionInfo{},
			procedures: map[string]rest.RESTProcedureInfo{},
		}
		var errs []string

		for name, scalar := range item.schema.ScalarTypes {
			if originScalar, ok := ndcSchema.ScalarTypes[name]; !ok {
				ndcSchema.ScalarTypes[name] = scalar
			} else if !rest.IsDefaultScalar(name) && !reflect.DeepEqual(originScalar, scalar) {
				slog.Warn(fmt.Sprintf("Scalar type %s is conflicted", name))
			}
		}
		for name, object := range item.schema.ObjectTypes {
			if _, ok := ndcSchema.ObjectTypes[name]; !ok {
				ndcSchema.ObjectTypes[name] = object
			} else {
				slog.Warn(fmt.Sprintf("Object type %s is conflicted", name))
			}
		}
		ndcSchema.Collections = append(ndcSchema.Collections, item.schema.Collections...)

		var functionSchemas []schema.FunctionInfo
		var procedureSchemas []schema.ProcedureInfo
		for _, fnItem := range item.schema.Functions {
			if fnItem.Request == nil || fnItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(fnItem.Request, "get")
			if err != nil {
				errs = append(errs, fmt.Sprintf("function %s: %s", fnItem.Name, err))
				continue
			}
			fn := rest.RESTFunctionInfo{
				Request:      req,
				FunctionInfo: fnItem.FunctionInfo,
			}
			meta.functions[fnItem.Name] = fn
			functionSchemas = append(functionSchemas, fn.FunctionInfo)
		}

		for _, procItem := range item.schema.Procedures {
			if procItem.Request == nil || procItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(procItem.Request, "")
			if err != nil {
				errs = append(errs, fmt.Sprintf("procedure %s: %s", procItem.Name, err))
				continue
			}
			meta.procedures[procItem.Name] = rest.RESTProcedureInfo{
				Request:       req,
				ProcedureInfo: procItem.ProcedureInfo,
			}
			procedureSchemas = append(procedureSchemas, procItem.ProcedureInfo)
		}

		if len(errs) > 0 {
			errors[item.name] = errs
			continue
		}
		ndcSchema.Functions = append(ndcSchema.Functions, functionSchemas...)
		ndcSchema.Procedures = append(ndcSchema.Procedures, procedureSchemas...)

		c.metadata = append(c.metadata, meta)
	}

	schemaBytes, err := json.Marshal(ndcSchema)
	if err != nil {
		errors["schema"] = []string{err.Error()}
	}

	if len(errors) > 0 {
		return errors
	}

	c.schema = &schema.SchemaResponse{
		ScalarTypes: ndcSchema.ScalarTypes,
		ObjectTypes: ndcSchema.ObjectTypes,
	}
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)
	return nil
}

func validateRequestSchema(req *rest.Request, defaultMethod string) (*rest.Request, error) {
	if req.Method == "" {
		if defaultMethod == "" {
			return nil, fmt.Errorf("the HTTP method is required")
		}
		req.Method = defaultMethod
	}

	if req.Type == "" {
		req.Type = rest.RequestTypeREST
	}

	return req, nil
}

func buildRESTArguments(restSchema *rest.NDCRestSchema, conf *ConfigItem) {
	if restSchema.Settings == nil || len(restSchema.Settings.Servers) < 2 {
		return
	}

	var serverIDs []string
	for i, server := range restSchema.Settings.Servers {
		if server.ID != "" {
			serverIDs = append(serverIDs, server.ID)
		} else {
			server.ID = fmt.Sprint(i)
			restSchema.Settings.Servers[i] = server
			serverIDs = append(serverIDs, server.ID)
		}
	}

	serverScalar := schema.NewScalarType()
	serverScalar.Representation = schema.NewTypeRepresentationEnum(serverIDs).Encode()

	restSchema.ScalarTypes[internal.RESTServerIDScalarName] = *serverScalar

	restOptionsObject := internal.RESTOptions{}
	restOptionsObjectType := restOptionsObject.SingleObjectType()
	restSchema.ObjectTypes[internal.RESTSingleOptionsObjectName] = *restOptionsObjectType

	restSingleOptionsArgument := schema.ArgumentInfo{
		Description: restOptionsObjectType.Description,
		Type:        schema.NewNullableNamedType(internal.RESTSingleOptionsObjectName).Encode(),
	}

	for _, fn := range restSchema.Functions {
		fn.FunctionInfo.Arguments[internal.RESTOptionsArgumentName] = restSingleOptionsArgument
	}

	for _, proc := range restSchema.Procedures {
		proc.ProcedureInfo.Arguments[internal.RESTOptionsArgumentName] = restSingleOptionsArgument
	}

	if !conf.Distributed {
		return
	}

	distributedObjectType := restOptionsObject.DistributedObjectType()
	restSchema.ObjectTypes[internal.RESTDistributedOptionsObjectName] = *distributedObjectType

	restDistributedArgument := schema.ArgumentInfo{
		Description: distributedObjectType.Description,
		Type:        schema.NewNullableNamedType(internal.RESTDistributedOptionsObjectName).Encode(),
	}

	restSchema.ObjectTypes[internal.DistributedErrorObjectName] = schema.ObjectType{
		Description: utils.ToPtr("The error response of the remote request"),
		Fields: schema.ObjectTypeFields{
			"server": schema.ObjectField{
				Description: utils.ToPtr("Identity of the remote server"),
				Type:        schema.NewNamedType(internal.RESTServerIDScalarName).Encode(),
			},
			"message": schema.ObjectField{
				Description: utils.ToPtr("An optional human-readable summary of the error"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarString))).Encode(),
			},
			"details": schema.ObjectField{
				Description: utils.ToPtr("Any additional structured information about the error"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
			},
		},
	}

	functionsLen := len(restSchema.Functions)
	for i := 0; i < functionsLen; i++ {
		fn := restSchema.Functions[i]
		funcName := buildDistributedName(fn.Name)
		info := schema.FunctionInfo{
			Arguments:   fn.FunctionInfo.Arguments,
			Description: fn.FunctionInfo.Description,
			Name:        funcName,
			ResultType:  schema.NewNamedType(buildDistributedResultObjectType(restSchema, funcName, fn.ResultType)).Encode(),
		}
		info.Arguments[internal.RESTOptionsArgumentName] = restDistributedArgument
		distributedFn := &rest.RESTFunctionInfo{
			Request:      fn.Request,
			FunctionInfo: info,
		}
		restSchema.Functions = append(restSchema.Functions, distributedFn)
	}

	proceduresLen := len(restSchema.Procedures)
	for i := 0; i < proceduresLen; i++ {
		proc := restSchema.Procedures[i]
		procName := buildDistributedName(proc.Name)
		info := schema.ProcedureInfo{
			Arguments:   proc.ProcedureInfo.Arguments,
			Description: proc.ProcedureInfo.Description,
			Name:        procName,
			ResultType:  schema.NewNamedType(buildDistributedResultObjectType(restSchema, procName, proc.ResultType)).Encode(),
		}
		info.Arguments[internal.RESTOptionsArgumentName] = restDistributedArgument

		distributedProc := &rest.RESTProcedureInfo{
			Request:       proc.Request,
			ProcedureInfo: info,
		}
		restSchema.Procedures = append(restSchema.Procedures, distributedProc)
	}
}

func buildDistributedResultObjectType(restSchema *rest.NDCRestSchema, operationName string, underlyingType schema.Type) string {
	distResultType := restUtils.StringSliceToPascalCase([]string{operationName, "Result"})
	distResultDataType := fmt.Sprintf("%sData", distResultType)

	restSchema.ObjectTypes[distResultDataType] = schema.ObjectType{
		Description: utils.ToPtr(fmt.Sprintf("Distributed response data of %s", operationName)),
		Fields: schema.ObjectTypeFields{
			"server": schema.ObjectField{
				Description: utils.ToPtr("Identity of the remote server"),
				Type:        schema.NewNamedType(internal.RESTServerIDScalarName).Encode(),
			},
			"data": schema.ObjectField{
				Description: utils.ToPtr(fmt.Sprintf("A result of %s", operationName)),
				Type:        underlyingType,
			},
		},
	}

	restSchema.ObjectTypes[distResultType] = schema.ObjectType{
		Description: utils.ToPtr(fmt.Sprintf("Distributed responses of %s", operationName)),
		Fields: schema.ObjectTypeFields{
			"results": schema.ObjectField{
				Description: utils.ToPtr(fmt.Sprintf("Results of %s", operationName)),
				Type:        schema.NewArrayType(schema.NewNamedType(distResultDataType)).Encode(),
			},
			"errors": schema.ObjectField{
				Description: utils.ToPtr(fmt.Sprintf("Error responses of %s", operationName)),
				Type:        schema.NewArrayType(schema.NewNamedType(internal.DistributedErrorObjectName)).Encode(),
			},
		},
	}

	return distResultType
}

func buildDistributedName(name string) string {
	return fmt.Sprintf("%sDistributed", name)
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}

func parseRESTOptionsFromArguments(arguments map[string]schema.ArgumentInfo, rawRestOptions any) (*internal.RESTOptions, error) {
	var result internal.RESTOptions
	if err := result.FromValue(rawRestOptions); err != nil {
		return nil, err
	}
	argInfo, ok := arguments[internal.RESTOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	restOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = restOptionsNamedType != nil && restOptionsNamedType.Name == internal.RESTDistributedOptionsObjectName
	return &result, nil
}
