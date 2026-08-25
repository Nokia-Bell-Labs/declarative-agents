// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"fmt"
	"math"
)

// ValidateBodySchema checks JSON-schema-lite properties and required fields.
func ValidateBodySchema(schema map[string]interface{}, payload map[string]interface{}) error {
	props, _ := schema["properties"].(map[string]interface{})
	required, _ := schema["required"].([]interface{})
	for _, raw := range required {
		field, _ := raw.(string)
		if _, ok := payload[field]; !ok {
			return fmt.Errorf("body field %q is required", field)
		}
	}
	for field, spec := range props {
		value, exists := payload[field]
		if !exists {
			continue
		}
		if err := validateJSONType(field, spec, value); err != nil {
			return err
		}
	}
	return nil
}

func validateBodySchema(schema map[string]interface{}, payload map[string]interface{}) error {
	return ValidateBodySchema(schema, payload)
}

func validateJSONType(field string, spec interface{}, value interface{}) error {
	rules, _ := spec.(map[string]interface{})
	want, _ := rules["type"].(string)
	if want == "" || jsonTypeMatches(want, value) {
		return nil
	}
	return fmt.Errorf("body field %q must be %s", field, want)
}

func jsonTypeMatches(want string, value interface{}) bool {
	switch want {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && math.Trunc(number) == number
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}
