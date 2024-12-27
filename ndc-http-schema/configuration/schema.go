package configuration

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// BuildSchemaFromConfig build NDC HTTP schema from the configuration
func BuildSchemaFromConfig(config *Configuration, configDir string, logger *slog.Logger) ([]NDCHttpRuntimeSchema, map[string][]string) {
	schemas := make([]NDCHttpRuntimeSchema, len(config.Files))
	errors := make(map[string][]string)
	existedFileIDs := []string{}

	for i, file := range config.Files {
		schemaOutput, err := buildSchemaFile(config, configDir, &file, logger)
		if err != nil {
			errors[file.File] = []string{err.Error()}
		}

		if schemaOutput == nil {
			continue
		}
		fileID := file.File
		if slices.Contains(existedFileIDs, fileID) {
			logger.Warn(fmt.Sprintf("the file %s is duplicated. Make sure that is intended", fileID))
			fileID += "_" + strconv.Itoa(i)
		}

		ndcSchema := NDCHttpRuntimeSchema{
			Name:          fileID,
			NDCHttpSchema: schemaOutput,
		}

		runtime, err := file.GetRuntimeSettings()
		if err != nil {
			errors[fileID] = []string{err.Error()}
		} else {
			ndcSchema.Runtime = *runtime
		}

		existedFileIDs = append(existedFileIDs, fileID)
		schemas[i] = ndcSchema
	}

	return schemas, errors
}

// ReadSchemaOutputFile reads the schema output file in disk
func ReadSchemaOutputFile(configDir string, filePath string, logger *slog.Logger) ([]NDCHttpRuntimeSchema, error) {
	if filePath == "" {
		return nil, nil
	}

	outputFilePath := filepath.Join(configDir, filePath)
	rawBytes, err := os.ReadFile(outputFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read the file at %s: %w", outputFilePath, err)
	}

	var result []NDCHttpRuntimeSchema
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the schema file at %s: %w", outputFilePath, err)
	}

	return result, nil
}

// MergeNDCHttpSchemas merge HTTP schemas into a single schema object
func MergeNDCHttpSchemas(config *Configuration, schemas []NDCHttpRuntimeSchema) (*rest.NDCHttpSchema, []NDCHttpRuntimeSchema, map[string][]string) {
	ndcSchema := &rest.NDCHttpSchema{
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
		ObjectTypes: make(map[string]rest.ObjectType),
		Functions:   make(map[string]rest.OperationInfo),
		Procedures:  make(map[string]rest.OperationInfo),
	}

	appliedSchemas := make([]NDCHttpRuntimeSchema, len(schemas))
	errors := make(map[string][]string)

	for i, item := range schemas {
		if item.NDCHttpSchema == nil {
			errors[item.Name] = []string{fmt.Sprintf("schema of the item %d (%s) is empty", i, item.Name)}

			return nil, nil, errors
		}
		settings := item.Settings
		if settings == nil {
			settings = &rest.NDCHttpSettings{}
		}

		meta := NDCHttpRuntimeSchema{
			Name:    item.Name,
			Runtime: item.Runtime,
			NDCHttpSchema: &rest.NDCHttpSchema{
				Settings:    settings,
				Functions:   map[string]rest.OperationInfo{},
				Procedures:  map[string]rest.OperationInfo{},
				ObjectTypes: item.ObjectTypes,
				ScalarTypes: item.ScalarTypes,
			},
		}
		var errs []string

		for name, scalar := range item.ScalarTypes {
			if originScalar, ok := ndcSchema.ScalarTypes[name]; !ok {
				ndcSchema.ScalarTypes[name] = scalar
			} else if !rest.IsDefaultScalar(name) && !reflect.DeepEqual(originScalar, scalar) {
				slog.Warn(fmt.Sprintf("Scalar type %s is conflicted", name))
			}
		}

		for name, object := range item.ObjectTypes {
			if _, ok := ndcSchema.ObjectTypes[name]; !ok {
				ndcSchema.ObjectTypes[name] = object
			} else {
				slog.Warn(fmt.Sprintf("Object type %s is conflicted", name))
			}
		}

		for fnName, fnItem := range item.Functions {
			if fnItem.Request == nil || fnItem.Request.URL == "" {
				continue
			}

			req, err := validateRequestSchema(fnItem.Request, "get")
			if err != nil {
				errs = append(errs, fmt.Sprintf("function %s: %s", fnName, err))

				continue
			}

			meta.Functions[fnName] = cloneOperationInfo(fnItem, req)
			ndcSchema.Functions[fnName] = cloneOperationInfo(fnItem, req)
		}

		for procName, procItem := range item.Procedures {
			if procItem.Request == nil || procItem.Request.URL == "" {
				continue
			}

			req, err := validateRequestSchema(procItem.Request, "")
			if err != nil {
				errs = append(errs, fmt.Sprintf("procedure %s: %s", procName, err))

				continue
			}

			meta.Procedures[procName] = cloneOperationInfo(procItem, req)
			ndcSchema.Procedures[procName] = cloneOperationInfo(procItem, req)
		}

		if len(errs) > 0 {
			errors[item.Name] = errs

			continue
		}
		appliedSchemas[i] = meta
	}

	return ndcSchema, appliedSchemas, errors
}

func buildSchemaFile(config *Configuration, configDir string, configItem *ConfigItem, logger *slog.Logger) (*rest.NDCHttpSchema, error) {
	if configItem.ConvertConfig.File == "" {
		return nil, errFilePathRequired
	}
	ResolveConvertConfigArguments(&configItem.ConvertConfig, configDir, nil)
	ndcSchema, err := ConvertToNDCSchema(&configItem.ConvertConfig, logger)
	if err != nil {
		return nil, err
	}

	if ndcSchema.Settings == nil || len(ndcSchema.Settings.Servers) == 0 {
		templates, err := getTemplates()
		if err != nil {
			return nil, err
		}
		if err := templates.ExecuteTemplate(os.Stderr, templateEmptySettings, map[string]any{
			"ContextPath": configDir,
			"Namespace":   configItem.ConvertConfig.File,
		}); err != nil {
			logger.Warn(err.Error())
		}

		return nil, fmt.Errorf("the servers setting of schema %s is empty", configItem.ConvertConfig.File)
	}

	// cache original result type to be used if header forwarding or distributed execution is enabled
	for key, op := range ndcSchema.Functions {
		op.OriginalResultType = op.ResultType
		ndcSchema.Functions[key] = op
	}

	for key, op := range ndcSchema.Procedures {
		op.OriginalResultType = op.ResultType
		ndcSchema.Procedures[key] = op
	}

	buildHTTPArguments(config, ndcSchema, configItem)
	buildHeadersForwardingResponse(config, ndcSchema)

	return ndcSchema, nil
}

func buildHTTPArguments(config *Configuration, restSchema *rest.NDCHttpSchema, conf *ConfigItem) {
	for _, fn := range restSchema.Functions {
		applyForwardingHeadersArgument(config, &fn)
	}

	for _, proc := range restSchema.Procedures {
		applyForwardingHeadersArgument(config, &proc)
	}

	if restSchema.Settings == nil || len(restSchema.Settings.Servers) < 2 {
		return
	}

	var serverIDs []string
	for i, server := range restSchema.Settings.Servers {
		if server.ID != "" {
			serverIDs = append(serverIDs, server.ID)
		} else {
			server.ID = strconv.Itoa(i)
			restSchema.Settings.Servers[i] = server
			serverIDs = append(serverIDs, server.ID)
		}
	}

	serverScalar := schema.NewScalarType()
	serverScalar.Representation = schema.NewTypeRepresentationEnum(serverIDs).Encode()

	restSchema.ScalarTypes[rest.HTTPServerIDScalarName] = *serverScalar
	restSchema.ObjectTypes[rest.HTTPSingleOptionsObjectName] = singleObjectType

	for _, fn := range restSchema.Functions {
		fn.Arguments[rest.HTTPOptionsArgumentName] = httpSingleOptionsArgument
	}

	for _, proc := range restSchema.Procedures {
		proc.Arguments[rest.HTTPOptionsArgumentName] = httpSingleOptionsArgument
	}

	if !conf.IsDistributed() {
		return
	}

	restSchema.ObjectTypes[rest.HTTPDistributedOptionsObjectName] = distributedObjectType
	restSchema.ObjectTypes[rest.DistributedErrorObjectName] = rest.ObjectType{
		Description: utils.ToPtr("The error response of the remote request"),
		Fields: map[string]rest.ObjectField{
			"server": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Identity of the remote server"),
					Type:        schema.NewNamedType(rest.HTTPServerIDScalarName).Encode(),
				},
			},
			"message": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("An optional human-readable summary of the error"),
					Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarString))).Encode(),
				},
			},
			"details": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Any additional structured information about the error"),
					Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
				},
			},
		},
	}

	functionKeys := utils.GetKeys(restSchema.Functions)
	for _, key := range functionKeys {
		fn := restSchema.Functions[key]
		funcName := buildDistributedName(key)
		distributedFn := rest.OperationInfo{
			Request:            fn.Request,
			Arguments:          cloneDistributedArguments(fn.Arguments),
			Description:        fn.Description,
			ResultType:         schema.NewNamedType(buildDistributedResultObjectType(restSchema, funcName, fn.ResultType)).Encode(),
			OriginalResultType: fn.OriginalResultType,
		}
		restSchema.Functions[funcName] = distributedFn
	}

	procedureKeys := utils.GetKeys(restSchema.Procedures)
	for _, key := range procedureKeys {
		proc := restSchema.Procedures[key]
		procName := buildDistributedName(key)

		distributedProc := rest.OperationInfo{
			Request:            proc.Request,
			Arguments:          cloneDistributedArguments(proc.Arguments),
			Description:        proc.Description,
			ResultType:         schema.NewNamedType(buildDistributedResultObjectType(restSchema, procName, proc.ResultType)).Encode(),
			OriginalResultType: proc.OriginalResultType,
		}
		restSchema.Procedures[procName] = distributedProc
	}
}

func buildHeadersForwardingResponse(config *Configuration, restSchema *rest.NDCHttpSchema) {
	if !config.ForwardHeaders.Enabled {
		return
	}

	if _, ok := restSchema.ScalarTypes[string(rest.ScalarJSON)]; !ok {
		restSchema.ScalarTypes[string(rest.ScalarJSON)] = schema.ScalarType{
			AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
			ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
			Representation:      schema.NewTypeRepresentationJSON().Encode(),
		}
	}

	if config.ForwardHeaders.ResponseHeaders == nil {
		return
	}

	for name, op := range restSchema.Functions {
		op.ResultType = createHeaderForwardingResponseTypes(restSchema, name, op.ResultType, config.ForwardHeaders.ResponseHeaders)
		restSchema.Functions[name] = op
	}
	for name, op := range restSchema.Procedures {
		op.ResultType = createHeaderForwardingResponseTypes(restSchema, name, op.ResultType, config.ForwardHeaders.ResponseHeaders)
		restSchema.Procedures[name] = op
	}
}

func applyForwardingHeadersArgument(config *Configuration, info *rest.OperationInfo) {
	if config.ForwardHeaders.Enabled && config.ForwardHeaders.ArgumentField != nil {
		info.Arguments[*config.ForwardHeaders.ArgumentField] = NewHeadersArgumentInfo()
	}
}

func cloneDistributedArguments(arguments map[string]rest.ArgumentInfo) map[string]rest.ArgumentInfo {
	result := map[string]rest.ArgumentInfo{}
	for k, v := range arguments {
		if k != rest.HTTPOptionsArgumentName {
			result[k] = v
		}
	}
	result[rest.HTTPOptionsArgumentName] = rest.ArgumentInfo{
		ArgumentInfo: schema.ArgumentInfo{
			Description: distributedObjectType.Description,
			Type:        schema.NewNullableNamedType(rest.HTTPDistributedOptionsObjectName).Encode(),
		},
	}

	return result
}

func buildDistributedResultObjectType(restSchema *rest.NDCHttpSchema, operationName string, underlyingType schema.Type) string {
	distResultType := restUtils.StringSliceToPascalCase([]string{operationName, "Result"})
	distResultDataType := distResultType + "Data"

	restSchema.ObjectTypes[distResultDataType] = rest.ObjectType{
		Description: utils.ToPtr("Distributed response data of " + operationName),
		Fields: map[string]rest.ObjectField{
			"server": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Identity of the remote server"),
					Type:        schema.NewNamedType(rest.HTTPServerIDScalarName).Encode(),
				},
			},
			DistributedObjectResultsDataKey: {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("A result of " + operationName),
					Type:        underlyingType,
				},
			},
		},
	}

	restSchema.ObjectTypes[distResultType] = rest.ObjectType{
		Description: utils.ToPtr("Distributed responses of " + operationName),
		Fields: map[string]rest.ObjectField{
			DistributedObjectResultsKey: {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Results of " + operationName),
					Type:        schema.NewArrayType(schema.NewNamedType(distResultDataType)).Encode(),
				},
			},
			DistributedObjectErrorsKey: {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Error responses of " + operationName),
					Type:        schema.NewArrayType(schema.NewNamedType(rest.DistributedErrorObjectName)).Encode(),
				},
			},
		},
	}

	return distResultType
}

func buildDistributedName(name string) string {
	return name + "Distributed"
}

func validateRequestSchema(req *rest.Request, defaultMethod string) (*rest.Request, error) {
	if req.Method == "" {
		if defaultMethod == "" {
			return nil, errHTTPMethodRequired
		}
		req.Method = defaultMethod
	}

	return req, nil
}

func createHeaderForwardingResponseTypes(restSchema *rest.NDCHttpSchema, operationName string, resultType schema.Type, settings *ForwardResponseHeadersSettings) schema.Type {
	objectName := restUtils.ToPascalCase(operationName) + "HeadersResponse"
	objectType := NewHeaderForwardingResponseObjectType(resultType, settings)

	restSchema.ObjectTypes[objectName] = objectType

	return schema.NewNamedType(objectName).Encode()
}

func cloneOperationInfo(operation rest.OperationInfo, req *rest.Request) rest.OperationInfo {
	args := make(map[string]rest.ArgumentInfo)
	for key, arg := range operation.Arguments {
		args[key] = arg
	}

	originalResultType := operation.OriginalResultType
	// if len(operation.OriginalResultType) == 0 {
	// 	originalResultType = operation.ResultType
	// }

	return rest.OperationInfo{
		Request:            req,
		Arguments:          args,
		Description:        operation.Description,
		ResultType:         operation.ResultType,
		OriginalResultType: originalResultType,
	}
}

// NewHeaderForwardingResponseObjectType creates a new type for header forwarding response.
func NewHeaderForwardingResponseObjectType(resultType schema.Type, settings *ForwardResponseHeadersSettings) rest.ObjectType {
	return rest.ObjectType{
		Fields: map[string]rest.ObjectField{
			settings.HeadersField: {
				ObjectField: schema.ObjectField{
					Type: schema.NewNullableNamedType(string(rest.ScalarJSON)).Encode(),
				},
			},
			settings.ResultField: {
				ObjectField: schema.ObjectField{
					Type: resultType,
				},
			},
		},
	}
}

// NewHeadersArgumentInfo creates a new forwarding headers argument information
func NewHeadersArgumentInfo() rest.ArgumentInfo {
	return rest.ArgumentInfo{
		ArgumentInfo: schema.ArgumentInfo{
			Description: utils.ToPtr("Headers forwarded from the Hasura engine"),
			Type:        schema.NewNullableNamedType(string(rest.ScalarJSON)).Encode(),
		},
	}
}
