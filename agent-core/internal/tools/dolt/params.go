// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var authorityFields = strings.Fields(`
	statement statements sql query dsn connection connection_ref database schema schema_statements
	commit_message commit_message_template operation operation_id kind host hostname port network socket
	user username password passwd tls tls_config tls_policy ssl cert certificate client_cert client_key ca
`)

type ParameterValidator struct {
	schema   *jsonschema.Schema
	declared map[string]bool
	required map[string]bool
}

func CompileParameterSchema(name string, declaration map[string]interface{}) (*ParameterValidator, error) {
	properties, ok := declaration["properties"].(map[string]interface{})
	if len(declaration) == 0 || declaration["type"] != "object" || !ok {
		return nil, fmt.Errorf("parameter_schema requires object type and properties")
	}
	declared := make(map[string]bool, len(properties))
	for property, raw := range properties {
		if !literalIdentifier.MatchString(property) || isAuthorityField(property) {
			return nil, fmt.Errorf("parameter_schema property %q is invalid or declares Dolt authority", property)
		}
		schema, ok := raw.(map[string]interface{})
		if !ok || schema["type"] == nil && schema["$ref"] == nil {
			return nil, fmt.Errorf("parameter_schema property %q must declare a type", property)
		}
		declared[property] = true
	}
	required, err := requiredNames(declaration["required"], declared)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	location := "urn:declarative-agents:dolt:" + name
	if err := compiler.AddResource(location, declaration); err != nil {
		return nil, fmt.Errorf("parameter_schema: %w", err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("parameter_schema: %w", err)
	}
	return &ParameterValidator{schema: schema, declared: declared, required: required}, nil
}

func requiredNames(raw interface{}, declared map[string]bool) (map[string]bool, error) {
	required := map[string]bool{}
	if raw == nil {
		return required, nil
	}
	var names []string
	switch values := raw.(type) {
	case []string:
		names = values
	case []interface{}:
		for _, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("parameter_schema required entries must be strings")
			}
			names = append(names, name)
		}
	default:
		return nil, fmt.Errorf("parameter_schema required must be an array")
	}
	for _, name := range names {
		if !declared[name] {
			return nil, fmt.Errorf("required parameter %q is not declared", name)
		}
		required[name] = true
	}
	return required, nil
}

func (v *ParameterValidator) Declared() map[string]bool { return maps.Clone(v.declared) }

func (v *ParameterValidator) ValidatePlaceholders(names []string) error {
	used := make(map[string]bool, len(names))
	for _, name := range names {
		if !v.declared[name] {
			return fmt.Errorf("statement placeholder %q is not declared by parameter_schema", name)
		}
		used[name] = true
	}
	var unused []string
	for name := range v.required {
		if !used[name] {
			unused = append(unused, name)
		}
	}
	if len(unused) > 0 {
		sort.Strings(unused)
		return fmt.Errorf("required parameters are unused by statement: %s", strings.Join(unused, ", "))
	}
	return nil
}

func (v *ParameterValidator) Validate(params map[string]interface{}) error {
	if params == nil {
		params = map[string]interface{}{}
	}
	if err := ValidateRuntimeInput(params); err != nil {
		return err
	}
	for name := range params {
		if !v.declared[name] {
			return fmt.Errorf("runtime parameter %q is not declared", name)
		}
	}
	if err := v.schema.Validate(params); err != nil {
		return fmt.Errorf("runtime parameters fail parameter_schema: %w", err)
	}
	return nil
}

func ValidateRuntimeInput(input map[string]interface{}) error {
	return rejectAuthority(input, "runtime input")
}

func rejectAuthority(value interface{}, path string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		for name, child := range typed {
			childPath := path + "." + name
			if isAuthorityField(name) {
				return fmt.Errorf("%s cannot set Dolt authority", childPath)
			}
			if err := rejectAuthority(child, childPath); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, child := range typed {
			if err := rejectAuthority(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func isAuthorityField(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "-", "_"))
	return slices.Contains(authorityFields, name)
}

type PreparedStatement struct {
	Query string
	Names []string
	Kind  StatementKind
}

func (s PreparedStatement) Bind(params map[string]interface{}) (string, []interface{}, error) {
	args := make([]interface{}, 0, len(s.Names))
	for _, name := range s.Names {
		value, ok := params[name]
		if !ok {
			return "", nil, fmt.Errorf("missing runtime parameter %q", name)
		}
		if structured(value) {
			encoded, err := json.Marshal(value)
			if err != nil {
				return "", nil, fmt.Errorf("encode runtime parameter %q: %w", name, err)
			}
			value = encoded
		}
		args = append(args, value)
	}
	return s.Query, args, nil
}

func structured(value interface{}) bool {
	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return true
	default:
		return false
	}
}

func (c *PreparedConfig) Bind(params map[string]interface{}) (string, []interface{}, error) {
	if err := c.Parameters.Validate(params); err != nil {
		return "", nil, err
	}
	return c.SQL.Bind(params)
}

var commitPlaceholder = regexp.MustCompile(
	`\{\{\s*(?:params\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`,
)

type CommitTemplate struct{ raw string }

func CompileCommitTemplate(raw string, declared map[string]bool) (*CommitTemplate, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	for _, match := range commitPlaceholder.FindAllStringSubmatch(raw, -1) {
		if !declared[match[1]] {
			return nil, fmt.Errorf("placeholder %q is not declared by parameter_schema", match[1])
		}
	}
	remainder := commitPlaceholder.ReplaceAllString(raw, "")
	if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
		return nil, fmt.Errorf("contains malformed placeholder")
	}
	return &CommitTemplate{raw: raw}, nil
}

func (t *CommitTemplate) Render(params map[string]interface{}) (string, error) {
	var renderErr error
	rendered := commitPlaceholder.ReplaceAllStringFunc(t.raw, func(match string) string {
		name := commitPlaceholder.FindStringSubmatch(match)[1]
		value, ok := params[name]
		if !ok {
			renderErr = fmt.Errorf("missing commit-message parameter %q", name)
			return ""
		}
		if text, ok := value.(string); ok {
			return text
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			renderErr = fmt.Errorf("render commit-message parameter %q: %w", name, err)
		}
		return string(encoded)
	})
	return rendered, renderErr
}

func (c *PreparedConfig) RenderCommitMessage(params map[string]interface{}) (string, error) {
	if c.CommitTemplate == nil {
		return "", fmt.Errorf("operation has no commit_message")
	}
	if err := c.Parameters.Validate(params); err != nil {
		return "", err
	}
	return c.CommitTemplate.Render(params)
}
