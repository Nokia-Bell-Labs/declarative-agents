// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

// DefinitionFile is the top-level YAML document for REST config files.
type DefinitionFile struct {
	Rest Definition `yaml:"rest"`
}

// Definition is the shared REST model used by hand-authored YAML and imports.
type Definition struct {
	Version           string                      `yaml:"version"`
	Clients           map[string]Client           `yaml:"clients,omitempty"`
	Servers           map[string]Server           `yaml:"servers,omitempty"`
	OpenAPI           map[string]OpenAPIImport    `yaml:"openapi,omitempty"`
	Auth              map[string]AuthProfile      `yaml:"auth,omitempty"`
	Limits            map[string]LimitProfile     `yaml:"limits,omitempty"`
	RetryPolicies     map[string]RetryPolicy      `yaml:"retry_policies,omitempty"`
	ResponseMappings  map[string]ResponseMapping  `yaml:"response_mappings,omitempty"`
	DocumentResources map[string]DocumentResource `yaml:"document_resources,omitempty"`
}
