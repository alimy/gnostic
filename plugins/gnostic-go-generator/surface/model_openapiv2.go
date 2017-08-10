// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnostic_surface_v1

import (
	"fmt"

	openapiv2 "github.com/googleapis/gnostic/OpenAPIv2"
)

// NewModelFromOpenAPI2 builds a model of an API service for use in code generation.
func NewModelFromOpenAPI2(document *openapiv2.Document) (*Model, error) {
	return newOpenAPI2Builder().buildModel(document)
}

type OpenAPI2Builder struct {
	model *Model
}

func newOpenAPI2Builder() *OpenAPI2Builder {
	return &OpenAPI2Builder{model: &Model{}}
}

func (b *OpenAPI2Builder) buildModel(document *openapiv2.Document) (*Model, error) {
	// Set model properties from passed-in document.
	b.model.Name = document.Info.Title
	b.model.Types = make([]*Type, 0)
	b.model.Methods = make([]*Method, 0)
	err := b.build(document)
	if err != nil {
		return nil, err
	}
	return b.model, nil
}

// buildV2 builds an API service description, preprocessing its types and methods for code generation.
func (b *OpenAPI2Builder) build(document *openapiv2.Document) (err error) {
	// Collect service type descriptions from Definitions section.
	if document.Definitions != nil {
		for _, pair := range document.Definitions.AdditionalProperties {
			t, err := b.buildTypeFromDefinition(pair.Name, pair.Value)
			if err != nil {
				return err
			}
			b.model.addType(t)
		}
	}
	// Collect service method descriptions from Paths section.
	for _, pair := range document.Paths.Path {
		v := pair.Value
		if v.Get != nil {
			b.buildMethodFromOperation(v.Get, "GET", pair.Name)
		}
		if v.Post != nil {
			b.buildMethodFromOperation(v.Post, "POST", pair.Name)
		}
		if v.Put != nil {
			b.buildMethodFromOperation(v.Put, "PUT", pair.Name)
		}
		if v.Delete != nil {
			b.buildMethodFromOperation(v.Delete, "DELETE", pair.Name)
		}
	}
	return err
}

func (b *OpenAPI2Builder) buildTypeFromDefinition(name string, schema *openapiv2.Schema) (t *Type, err error) {
	t = &Type{}
	t.Name = name
	t.Description = "implements the service definition of " + name
	t.Fields = make([]*Field, 0)
	if schema.Properties != nil {
		if len(schema.Properties.AdditionalProperties) > 0 {
			// If the schema has properties, generate a struct.
			t.Kind = Kind_STRUCT
		}
		for _, pair2 := range schema.Properties.AdditionalProperties {
			var f Field
			f.Name = pair2.Name
			f.Type = b.typeForSchema(pair2.Value)
			f.Serialize = true
			t.addField(&f)
		}
	}
	if len(t.Fields) == 0 {
		if schema.AdditionalProperties != nil {
			// If the schema has no fixed properties and additional properties of a specified type,
			// generate a map pointing to objects of that type.
			mapType := typeForRef(schema.AdditionalProperties.GetSchema().XRef)
			t.Kind = Kind_MAP
			t.MapType = mapType
		}
	}
	return t, err
}

func (b *OpenAPI2Builder) buildMethodFromOperation(op *openapiv2.Operation, method string, path string) (err error) {
	var m Method
	m.Operation = op.OperationId
	m.Path = path
	m.Method = method
	m.Description = op.Description

	m.Name = sanitizeOperationName(op.OperationId)
	if m.Name == "" {
		m.Name = generateOperationName(method, path)
	}

	m.ParametersTypeName, err = b.buildTypeFromParameters(m.Name, op.Parameters)
	m.ResponsesTypeName, err = b.buildTypeFromResponses(&m, m.Name, op.Responses)
	b.model.addMethod(&m)
	return err
}

func (b *OpenAPI2Builder) buildTypeFromParameters(name string, parameters []*openapiv2.ParametersItem) (typeName string, err error) {
	t := &Type{}
	t.Name = name + "Parameters"
	t.Description = t.Name + " holds parameters to " + name
	t.Kind = Kind_STRUCT
	t.Fields = make([]*Field, 0)
	for _, parametersItem := range parameters {
		var f Field
		f.Type = fmt.Sprintf("%+v", parametersItem)
		parameter := parametersItem.GetParameter()
		if parameter != nil {
			bodyParameter := parameter.GetBodyParameter()
			if bodyParameter != nil {
				f.Name = bodyParameter.Name
				if bodyParameter.Schema != nil {
					f.Type = b.typeForSchema(bodyParameter.Schema)
				}
				f.Position = Position_BODY
			}
			nonBodyParameter := parameter.GetNonBodyParameter()
			if nonBodyParameter != nil {
				headerParameter := nonBodyParameter.GetHeaderParameterSubSchema()
				if headerParameter != nil {
					f.Name = headerParameter.Name
					f.Type = headerParameter.Type
					f.Position = Position_HEADER
				}
				formDataParameter := nonBodyParameter.GetFormDataParameterSubSchema()
				if formDataParameter != nil {
					f.Name = formDataParameter.Name
					f.Type = formDataParameter.Type
					f.Position = Position_FORMDATA
				}
				queryParameter := nonBodyParameter.GetQueryParameterSubSchema()
				if queryParameter != nil {
					f.Name = queryParameter.Name
					f.Type = queryParameter.Type
					f.Position = Position_QUERY
				}
				pathParameter := nonBodyParameter.GetPathParameterSubSchema()
				if pathParameter != nil {
					f.Name = pathParameter.Name
					f.Type = pathParameter.Type
					f.Format = pathParameter.Format
					f.Position = Position_PATH
				}
			}
			f.Serialize = true
			t.addField(&f)
		}
	}
	if len(t.Fields) > 0 {
		b.model.addType(t)
		return t.Name, err
	}
	return "", err
}

func (b *OpenAPI2Builder) buildTypeFromResponses(m *Method, name string, responses *openapiv2.Responses) (typeName string, err error) {
	t := &Type{}
	t.Name = name + "Responses"
	t.Description = t.Name + " holds responses of " + name
	t.Kind = Kind_STRUCT
	t.Fields = make([]*Field, 0)

	for _, responseCode := range responses.ResponseCode {
		var f Field
		f.Name = responseCode.Name
		f.Serialize = false
		response := responseCode.Value.GetResponse()
		if response != nil && response.Schema != nil && response.Schema.GetSchema() != nil {
			f.ValueType = b.typeForSchema(response.Schema.GetSchema())
			f.Type = "*" + f.ValueType
			t.addField(&f)
		}
	}

	if len(t.Fields) > 0 {
		b.model.addType(t)
		return t.Name, err
	}
	return "", err
}

func (b *OpenAPI2Builder) typeForSchema(schema *openapiv2.Schema) (typeName string) {
	ref := schema.XRef
	if ref != "" {
		return typeForRef(ref)
	}
	if schema.Type != nil {
		types := schema.Type.Value
		format := schema.Format
		if len(types) == 1 && types[0] == "string" {
			return "string"
		}
		if len(types) == 1 && types[0] == "integer" && format == "int32" {
			return "int32"
		}
		if len(types) == 1 && types[0] == "integer" {
			return "int"
		}
		if len(types) == 1 && types[0] == "number" {
			return "int"
		}
		if len(types) == 1 && types[0] == "array" && schema.Items != nil {
			// we have an array.., but of what?
			items := schema.Items.Schema
			if len(items) == 1 && items[0].XRef != "" {
				return "[]" + typeForRef(items[0].XRef)
			}
		}
		if len(types) == 1 && types[0] == "object" && schema.AdditionalProperties == nil {
			return "map[string]interface{}"
		}
	}
	if schema.AdditionalProperties != nil {
		additionalProperties := schema.AdditionalProperties
		if propertySchema := additionalProperties.GetSchema(); propertySchema != nil {
			if ref := propertySchema.XRef; ref != "" {
				return "map[string]" + typeForRef(ref)
			}
		}
	}
	// this function is incomplete... so return a string representing anything that we don't handle
	return fmt.Sprintf("%v", schema)
}
