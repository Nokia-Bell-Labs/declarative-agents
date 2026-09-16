// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"

// DefinitionFile is the top-level YAML document for REST config files.
type DefinitionFile struct {
	Unit           string                    `yaml:"unit,omitempty"`
	Imports        []string                  `yaml:"imports,omitempty"`
	Params         []fragments.Param         `yaml:"params,omitempty"`
	Instantiate    []fragments.Instantiation `yaml:"instantiate,omitempty"`
	Rest           Definition                `yaml:"rest"`
	hasRest        bool
	hasImports     bool
	hasParams      bool
	hasInstantiate bool
}

// IsFragment reports a REST file with declared parameters, which is only ever
// instantiated and never imported plainly (srd052 R1.1, R2.6).
func (f DefinitionFile) IsFragment() bool { return f.hasParams }

// Definition is the shared REST model used by hand-authored YAML and imports.
type Definition struct {
	Version            string                      `yaml:"version"`
	Clients            map[string]Client           `yaml:"clients,omitempty"`
	Servers            map[string]Server           `yaml:"servers,omitempty"`
	OpenAPI            map[string]OpenAPIImport    `yaml:"openapi,omitempty"`
	Auth               map[string]AuthProfile      `yaml:"auth,omitempty"`
	Limits             map[string]LimitProfile     `yaml:"limits,omitempty"`
	RetryPolicies      map[string]RetryPolicy      `yaml:"retry_policies,omitempty"`
	ResponseMappings   map[string]ResponseMapping  `yaml:"response_mappings,omitempty"`
	DocumentResources  map[string]DocumentResource `yaml:"document_resources,omitempty"`
	declarationSources map[string]map[string]DeclarationSource
	declarationImports []DeclarationImport
	instantiations     []DeclarationInstantiation
	openAPIConsumers   map[string]map[string]bool
}
