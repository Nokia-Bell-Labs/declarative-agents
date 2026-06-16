// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func readRequestPayload(req *http.Request, endpoint Endpoint, maxBytes int) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if err := addQueryValues(payload, endpoint.Request.Query, req.URL.Query()); err != nil {
		return nil, err
	}
	if err := addHeaderValues(payload, endpoint.Request.Headers, req.Header); err != nil {
		return nil, err
	}
	if maxBytes > 0 {
		req.Body = http.MaxBytesReader(nil, req.Body, int64(maxBytes))
	}
	return readRequestBody(payload, req, endpointBodySchema(endpoint))
}

func readRequestBody(payload map[string]interface{}, req *http.Request, bodySchema map[string]interface{}) (map[string]interface{}, error) {
	if len(bodySchema) == 0 {
		return payload, nil
	}
	body := map[string]interface{}{}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if err := validateBodySchema(bodySchema, body); err != nil {
		return nil, err
	}
	payload["body"] = body
	for key, value := range body {
		payload[key] = value
	}
	return payload, nil
}

func endpointBodySchema(endpoint Endpoint) map[string]interface{} {
	if len(endpoint.Request.BodySchema) > 0 {
		return endpoint.Request.BodySchema
	}
	if endpoint.Binding == bindingLifecycleControl {
		return endpoint.LifecycleControl.TargetSchema
	}
	return nil
}

func addPathValues(payload map[string]interface{}, schema map[string]interface{}, vars map[string]string) error {
	path := map[string]interface{}{}
	for name, value := range vars {
		typed, err := validateStringValue("path param", name, schema[name], value)
		if err != nil {
			return err
		}
		path[name] = typed
		payload[name] = typed
	}
	payload["path"] = path
	return nil
}

func addQueryValues(payload map[string]interface{}, schema map[string]interface{}, values map[string][]string) error {
	query := map[string]interface{}{}
	for name, raw := range values {
		if _, ok := schema[name]; !ok {
			return fmt.Errorf("query param %q is not declared", name)
		}
		typed, err := validateStringValue("query param", name, schema[name], firstValue(raw))
		if err != nil {
			return err
		}
		query[name] = typed
		payload[name] = typed
	}
	payload["query"] = query
	return nil
}

func addHeaderValues(payload map[string]interface{}, schema map[string]interface{}, values http.Header) error {
	headers := map[string]interface{}{}
	for name, raw := range values {
		field := strings.ToLower(name)
		spec, declared := lookupHeaderSchema(schema, field)
		if !declared {
			if allowedUndeclaredHeaders[field] {
				continue
			}
			return fmt.Errorf("header %q is not declared", field)
		}
		typed, err := validateStringValue("header", field, spec, firstValue(raw))
		if err != nil {
			return err
		}
		headers[field] = typed
		payload[field] = typed
	}
	payload["headers"] = headers
	return nil
}

func lookupHeaderSchema(schema map[string]interface{}, field string) (interface{}, bool) {
	for name, spec := range schema {
		if strings.EqualFold(name, field) {
			return spec, true
		}
	}
	return nil, false
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func validateStringValue(kind, name string, spec interface{}, value string) (interface{}, error) {
	rules, _ := spec.(map[string]interface{})
	switch want, _ := rules["type"].(string); want {
	case "", "string":
		return value, nil
	case "integer":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be integer", kind, name)
		}
		return parsed, nil
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be number", kind, name)
		}
		return parsed, nil
	case "boolean":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be boolean", kind, name)
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func writeRequestError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}
