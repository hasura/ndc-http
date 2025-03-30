package ndc

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
)

// ConvertOptions represent the common convert options for both OpenAPI v2 and v3.
type ConvertOptions struct {
	Prefix string
	Logger *slog.Logger
}

// BuildNDCSchema validates and builds the NDC schema.
func BuildNDCSchema(input []byte, options ConvertOptions) (*rest.NDCHttpSchema, error) {
	var result *rest.NDCHttpSchema

	if err := json.Unmarshal(input, &result); err != nil {
		return nil, err
	}

	return NewNDCBuilder(result, options).Build()
}

// NDCBuilder the NDC schema builder to validate REST connector schema.
type NDCBuilder struct {
	*ConvertOptions

	schema      *rest.NDCHttpSchema
	newSchema   *rest.NDCHttpSchema
	usedTypes   map[string]string
	bannedNames map[string]string
}

// NewNDCBuilder creates a new NDCBuilder instance.
func NewNDCBuilder(httpSchema *rest.NDCHttpSchema, options ConvertOptions) *NDCBuilder {
	newSchema := rest.NewNDCHttpSchema()
	newSchema.Settings = httpSchema.Settings

	return &NDCBuilder{
		ConvertOptions: &options,
		usedTypes:      make(map[string]string),
		schema:         httpSchema,
		newSchema:      newSchema,
		bannedNames:    make(map[string]string),
	}
}

// Build validates and build the REST connector schema.
func (ndc *NDCBuilder) Build() (*rest.NDCHttpSchema, error) {
	if err := ndc.validate(); err != nil {
		return nil, err
	}

	return ndc.newSchema, nil
}

// Validate checks if the schema is valid.
func (nsc *NDCBuilder) validate() error {
	if err := nsc.validateBannedTypes(); err != nil {
		return err
	}

	for key, operation := range nsc.schema.Functions {
		op, err := nsc.validateOperation(key, operation)
		if err != nil {
			return err
		}

		newName := nsc.formatOperationName(key)
		nsc.newSchema.Functions[newName] = *op
	}

	for key, operation := range nsc.schema.Procedures {
		op, err := nsc.validateOperation(key, operation)
		if err != nil {
			return err
		}

		newName := nsc.formatOperationName(key)
		nsc.newSchema.Procedures[newName] = *op
	}

	return nil
}

// recursively validate and clean unused objects as well as their inner properties.
func (nsc *NDCBuilder) validateOperation(operationName string, operation rest.OperationInfo) (*rest.OperationInfo, error) {
	result := &rest.OperationInfo{
		Request:     operation.Request,
		Description: operation.Description,
		Arguments:   make(map[string]rest.ArgumentInfo),
	}

	for key, field := range operation.Arguments {
		fieldType, err := nsc.validateType(field.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: arguments.%s: %w", operationName, key, err)
		}
		result.Arguments[key] = rest.ArgumentInfo{
			HTTP: field.HTTP,
			ArgumentInfo: schema.ArgumentInfo{
				Description: field.ArgumentInfo.Description,
				Type:        fieldType.Encode(),
			},
		}
	}

	resultType, err := nsc.validateType(operation.ResultType)
	if err != nil {
		return nil, fmt.Errorf("%s: result_type: %w", operationName, err)
	}

	result.ResultType = resultType.Encode()

	return result, nil
}

// recursively validate used types as well as their inner properties.
func (nsc *NDCBuilder) validateType(schemaType schema.Type) (schema.TypeEncoder, error) {
	rawType, err := schemaType.InterfaceT()

	switch t := rawType.(type) {
	case *schema.NullableType:
		underlyingType, err := nsc.validateType(t.UnderlyingType)
		if err != nil {
			return nil, err
		}

		return utils.WrapNullableTypeEncoder(underlyingType), nil
	case *schema.ArrayType:
		elementType, err := nsc.validateType(t.ElementType)
		if err != nil {
			return nil, err
		}

		return schema.NewArrayType(elementType), nil
	case *schema.NamedType:
		if t.Name == "" {
			return nil, errors.New("named type is empty")
		}

		name, isBannedName := nsc.bannedNames[strings.ToLower(t.Name)]
		if !isBannedName {
			name = t.Name
		}

		if usedName, ok := nsc.usedTypes[name]; ok {
			return schema.NewNamedType(usedName), nil
		}

		if st, ok := nsc.schema.ScalarTypes[name]; ok {
			newName := name

			if !rest.IsDefaultScalar(newName) && !isBannedName {
				newName = nsc.formatTypeName(t.Name)
			}

			newNameType := schema.NewNamedType(newName)
			nsc.usedTypes[name] = newName

			if _, ok := nsc.newSchema.ScalarTypes[newName]; !ok {
				nsc.newSchema.ScalarTypes[newName] = st
			}

			return newNameType, nil
		}

		objectType, ok := nsc.schema.ObjectTypes[name]
		if !ok {
			return nil, errors.New(name + ": named type does not exist")
		}

		newName := nsc.formatTypeName(name)
		newNameType := schema.NewNamedType(newName)
		nsc.usedTypes[name] = newName

		newObjectType := rest.ObjectType{
			Alias:       objectType.Alias,
			Description: objectType.Description,
			XML:         objectType.XML,
			Fields:      make(map[string]rest.ObjectField),
		}

		for key, field := range objectType.Fields {
			fieldType, err := nsc.validateType(field.Type)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.Name, key, err)
			}

			newObjectType.Fields[key] = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type:        fieldType.Encode(),
					Description: field.Description,
					Arguments:   field.Arguments,
				},
				HTTP: field.HTTP,
			}
		}

		nsc.newSchema.ObjectTypes[newName] = newObjectType

		return newNameType, nil
	default:
		return nil, err
	}
}

func (nsc *NDCBuilder) formatTypeName(name string) string {
	if nsc.Prefix == "" {
		return name
	}

	return utils.StringSliceToPascalCase([]string{nsc.Prefix, name})
}

func (nsc *NDCBuilder) formatOperationName(name string) string {
	if nsc.Prefix == "" {
		return name
	}

	return utils.StringSliceToCamelCase([]string{nsc.Prefix, name})
}

func (nsc *NDCBuilder) validateBannedTypes() error {
	for key, obj := range nsc.schema.ObjectTypes {
		lowerKey := strings.ToLower(key)

		for scalarKey, scalarType := range nsc.schema.ScalarTypes {
			if lowerKey == strings.ToLower(scalarKey) {
				err := fmt.Errorf("the insensitive name `%s` exists in both object and scalar types", key)
				nsc.Logger.Error(err.Error(), slog.Any("object_type", obj), slog.Any("scalar_type", scalarType))

				return err
			}
		}

		if !slices.Contains(bannedTypeNames, lowerKey) {
			continue
		}

		newName := key + "Object"
		nsc.bannedNames[lowerKey] = newName
		nsc.schema.ObjectTypes[newName] = obj
		delete(nsc.schema.ObjectTypes, key)
	}

	for key, scalarType := range nsc.schema.ScalarTypes {
		lowerKey := strings.ToLower(key)

		if !slices.Contains(bannedTypeNames, lowerKey) {
			continue
		}

		newName := key + "Scalar"
		nsc.bannedNames[lowerKey] = newName
		nsc.schema.ScalarTypes[newName] = scalarType
		delete(nsc.schema.ScalarTypes, key)
	}

	return nil
}
