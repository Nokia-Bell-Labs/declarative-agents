// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type openAPIOperation struct {
	id        string
	source    string
	method    string
	path      string
	operation *openapi3.Operation
}

// CompileOpenAPIImports loads OpenAPI imports into the internal REST model.
func CompileOpenAPIImports(def *Definition, baseDir string) error {
	if len(def.OpenAPI) == 0 {
		return nil
	}
	imports := def.OpenAPI
	for name, imp := range imports {
		operations, err := loadOpenAPIOperations(name, imp, baseDir)
		if err != nil {
			return err
		}
		if err := applyOpenAPIExpose(def, name, imp, operations); err != nil {
			return err
		}
		if err := applyOpenAPIBind(def, name, imp, operations); err != nil {
			return err
		}
		if err := applyOpenAPIRefs(def, name, operations); err != nil {
			return err
		}
	}
	def.OpenAPI = nil
	return nil
}

func loadOpenAPIOperations(name string, imp OpenAPIImport, baseDir string) (map[string]openAPIOperation, error) {
	doc, err := loadOpenAPIDocument(imp, baseDir)
	if err != nil {
		return nil, fmt.Errorf("openapi %q source %q: %w", name, imp.Path, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("openapi %q source %q validation: %w", name, imp.Path, err)
	}
	return indexOpenAPIOperations(name, imp.Path, doc)
}

func loadOpenAPIDocument(imp OpenAPIImport, baseDir string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	if filepath.IsAbs(imp.Path) {
		return loader.LoadFromFile(imp.Path)
	}
	return loader.LoadFromFile(filepath.Join(baseDir, imp.Path))
}

func indexOpenAPIOperations(name, source string, doc *openapi3.T) (map[string]openAPIOperation, error) {
	index := map[string]openAPIOperation{}
	for _, path := range doc.Paths.InMatchingOrder() {
		pathItem := doc.Paths.Value(path)
		for method, operation := range pathItem.Operations() {
			id := operation.OperationID
			if id == "" {
				continue
			}
			if previous, ok := index[id]; ok {
				return nil, fmt.Errorf("openapi %q operation_id %q is duplicated in %s and %s", name, id, previous.source, path)
			}
			index[id] = openAPIOperation{id: id, source: source, method: strings.ToUpper(method), path: path, operation: operation}
		}
	}
	return index, nil
}

func applyOpenAPIExpose(
	def *Definition,
	name string,
	imp OpenAPIImport,
	operations map[string]openAPIOperation,
) error {
	if len(imp.Expose) == 0 {
		return nil
	}
	client := def.Clients[name]
	if client.BaseURL == "" {
		client.BaseURL = imp.BaseURL
	}
	if client.Operations == nil {
		client.Operations = map[string]Operation{}
	}
	for _, operationID := range imp.Expose {
		compiled, err := compiledOperation(name, imp, operationID, operations)
		if err != nil {
			return err
		}
		client.Operations[operationID] = compiled
	}
	if def.Clients == nil {
		def.Clients = map[string]Client{}
	}
	def.Clients[name] = client
	return nil
}

func applyOpenAPIBind(
	def *Definition,
	name string,
	imp OpenAPIImport,
	operations map[string]openAPIOperation,
) error {
	for operationID, endpointName := range imp.Bind {
		compiled, err := endpointFromOpenAPI(name, operationID, endpointName, operations)
		if err != nil {
			return err
		}
		if err := setBoundEndpoint(def, endpointName, compiled); err != nil {
			return err
		}
	}
	return nil
}

func applyOpenAPIRefs(def *Definition, name string, operations map[string]openAPIOperation) error {
	for clientName, client := range def.Clients {
		if err := applyClientOpenAPIRefs(&client, name, operations); err != nil {
			return err
		}
		def.Clients[clientName] = client
	}
	for serverName, server := range def.Servers {
		if err := applyServerOpenAPIRefs(&server, name, operations); err != nil {
			return err
		}
		def.Servers[serverName] = server
	}
	return nil
}

func applyClientOpenAPIRefs(client *Client, name string, operations map[string]openAPIOperation) error {
	for opName, operation := range client.Operations {
		compiled, err := mergeOpenAPIOperation(name, operation, operations)
		if err != nil {
			return err
		}
		client.Operations[opName] = compiled
	}
	for resName, resource := range client.Resources {
		for opName, operation := range resource.Operations {
			compiled, err := mergeOpenAPIOperation(name, operation, operations)
			if err != nil {
				return err
			}
			resource.Operations[opName] = compiled
		}
		client.Resources[resName] = resource
	}
	return nil
}

func applyServerOpenAPIRefs(server *Server, name string, operations map[string]openAPIOperation) error {
	for endpointName, endpoint := range server.Endpoints {
		compiled, err := mergeOpenAPIEndpoint(name, endpoint, operations)
		if err != nil {
			return err
		}
		server.Endpoints[endpointName] = compiled
	}
	return nil
}

func compiledOperation(
	name string,
	imp OpenAPIImport,
	operationID string,
	operations map[string]openAPIOperation,
) (Operation, error) {
	found, err := requireOpenAPIOperation(name, operationID, operations)
	if err != nil {
		return Operation{}, err
	}
	operation := Operation{OpenAPIOperationID: operationID, Success: defaultOpenAPISuccess(found.method)}
	operation.SideEffects = imp.SideEffects[operationID]
	operation.Reversibility = imp.Reversibility[operationID]
	return mergeCompiledOperation(operation, found), nil
}

func mergeOpenAPIOperation(
	name string,
	operation Operation,
	operations map[string]openAPIOperation,
) (Operation, error) {
	if operation.OpenAPIOperationID == "" {
		return operation, nil
	}
	found, err := requireOpenAPIOperation(name, operation.OpenAPIOperationID, operations)
	if err != nil {
		return Operation{}, err
	}
	return mergeCompiledOperation(operation, found), nil
}

func mergeCompiledOperation(operation Operation, found openAPIOperation) Operation {
	operation.Method = found.method
	operation.Path = found.path
	operation.Params = requestBindingFromOpenAPI(found.operation)
	if len(operation.Response.Schema) == 0 {
		operation.Response.Schema = responseSchemaFromOpenAPI(found.operation)
	}
	return operation
}

func endpointFromOpenAPI(
	name string,
	operationID string,
	endpointName string,
	operations map[string]openAPIOperation,
) (Endpoint, error) {
	if endpointName == "" {
		return Endpoint{}, fmt.Errorf("openapi %q operation_id %q binds an empty endpoint name", name, operationID)
	}
	found, err := requireOpenAPIOperation(name, operationID, operations)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{
		OpenAPIOperationID: operationID, Method: found.method, Path: found.path,
		Request:  requestBindingFromOpenAPI(found.operation),
		Response: ResponseMapping{Schema: responseSchemaFromOpenAPI(found.operation)},
	}, nil
}

func mergeOpenAPIEndpoint(name string, endpoint Endpoint, operations map[string]openAPIOperation) (Endpoint, error) {
	if endpoint.OpenAPIOperationID == "" {
		return endpoint, nil
	}
	found, err := requireOpenAPIOperation(name, endpoint.OpenAPIOperationID, operations)
	if err != nil {
		return Endpoint{}, err
	}
	endpoint.Method = found.method
	endpoint.Path = found.path
	endpoint.Request = requestBindingFromOpenAPI(found.operation)
	if len(endpoint.Response.Schema) == 0 {
		endpoint.Response.Schema = responseSchemaFromOpenAPI(found.operation)
	}
	return endpoint, nil
}

func setBoundEndpoint(def *Definition, endpointName string, compiled Endpoint) error {
	for serverName, server := range def.Servers {
		endpoint, ok := server.Endpoints[endpointName]
		if !ok {
			continue
		}
		if endpoint.OpenAPIOperationID != "" && endpoint.OpenAPIOperationID != compiled.OpenAPIOperationID {
			return fmt.Errorf("openapi bind endpoint %q has incompatible operation_id %q", endpointName, endpoint.OpenAPIOperationID)
		}
		compiled.Binding = endpoint.Binding
		compiled.Signal = endpoint.Signal
		compiled.AllowedSignals = endpoint.AllowedSignals
		compiled.Queue = endpoint.Queue
		server.Endpoints[endpointName] = compiled
		def.Servers[serverName] = server
		return nil
	}
	return fmt.Errorf("openapi bind endpoint %q is not configured", endpointName)
}

func requireOpenAPIOperation(name, operationID string, operations map[string]openAPIOperation) (openAPIOperation, error) {
	operation, ok := operations[operationID]
	if !ok {
		return openAPIOperation{}, fmt.Errorf("openapi %q operation_id %q is missing", name, operationID)
	}
	return operation, nil
}

func defaultOpenAPISuccess(method string) StatusMapping {
	signal := "RESTResponded"
	if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
		signal = "RESTResourceWritten"
	}
	return StatusMapping{Status: []int{200, 201, 202, 204}, Signal: signal}
}

func requestBindingFromOpenAPI(operation *openapi3.Operation) RequestBinding {
	binding := RequestBinding{
		Path: map[string]interface{}{}, Query: map[string]interface{}{}, Headers: map[string]interface{}{},
	}
	for _, ref := range operation.Parameters {
		addOpenAPIParameter(binding, ref)
	}
	if schema := requestBodySchema(operation); schema != nil {
		binding.BodySchema = schemaMap(schema.Value)
	}
	return binding
}

func addOpenAPIParameter(binding RequestBinding, ref *openapi3.ParameterRef) {
	if ref == nil || ref.Value == nil {
		return
	}
	schema := schemaMap(nil)
	if ref.Value.Schema != nil {
		schema = schemaMap(ref.Value.Schema.Value)
	}
	switch ref.Value.In {
	case "path":
		binding.Path[ref.Value.Name] = schema
	case "query":
		binding.Query[ref.Value.Name] = schema
	case "header":
		binding.Headers[ref.Value.Name] = schema
	}
}

func requestBodySchema(operation *openapi3.Operation) *openapi3.SchemaRef {
	if operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return nil
	}
	return schemaFromContent(operation.RequestBody.Value.Content)
}

func responseSchemaFromOpenAPI(operation *openapi3.Operation) map[string]interface{} {
	if operation.Responses == nil {
		return nil
	}
	for _, status := range operation.Responses.Keys() {
		response := operation.Responses.Value(status)
		if response != nil && response.Value != nil {
			schema := schemaFromContent(response.Value.Content)
			if schema != nil {
				return schemaMap(schema.Value)
			}
		}
	}
	if response := operation.Responses.Default(); response != nil && response.Value != nil {
		schema := schemaFromContent(response.Value.Content)
		if schema != nil {
			return schemaMap(schema.Value)
		}
	}
	return nil
}

func schemaFromContent(content openapi3.Content) *openapi3.SchemaRef {
	if media := content.Get("application/json"); media != nil {
		return media.Schema
	}
	for _, mediaType := range content {
		return mediaType.Schema
	}
	return nil
}

func schemaMap(schema *openapi3.Schema) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{}
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}
