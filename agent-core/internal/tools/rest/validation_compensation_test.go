// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDefinitionAcceptsExecutableCompensationTargets(t *testing.T) {
	t.Parallel()

	t.Run("resource operation with resource id alias", func(t *testing.T) {
		t.Parallel()
		create := compensationSourceOperation("delete")
		create.Path = "/things/{number}"
		create.Params.Path = map[string]interface{}{"number": map[string]interface{}{"type": "string"}}
		deleteOp := irreversibleOperation("DELETE", "/things/{id}")
		deleteOp.Params.Path = map[string]interface{}{"id": map[string]interface{}{"type": "string"}}
		def := Definition{
			Version: "v1",
			Clients: map[string]Client{"api": {
				Resources: map[string]Resource{"thing": {
					Path: "/things/{number}",
					Operations: map[string]Operation{
						"create": create,
						"delete": deleteOp,
					},
				}},
			}},
		}

		require.NoError(t, ValidateDefinition(def))
	})

	t.Run("top-level operation", func(t *testing.T) {
		t.Parallel()
		create := compensationSourceOperation("delete_thing")
		create.ResponseRef = "created"
		deleteOp := irreversibleOperation("DELETE", "/things/{id}")
		deleteOp.Params.Path = map[string]interface{}{"id": map[string]interface{}{"type": "string"}}
		def := Definition{
			Version: "v1",
			ResponseMappings: map[string]ResponseMapping{
				"created": {ResourceID: "$.id"},
			},
			Clients: map[string]Client{"api": {
				Operations: map[string]Operation{
					"create_thing": create,
					"delete_thing": deleteOp,
				},
			}},
		}

		require.NoError(t, ValidateDefinition(def))
	})
}

func TestValidateDefinitionRejectsInvalidCompensationTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Operation)
		wantErr string
	}{
		{
			name: "missing compensation",
			mutate: func(operation *Operation) {
				operation.Compensation = nil
			},
			wantErr: "without an executable compensation target",
		},
		{
			name: "missing target",
			mutate: func(operation *Operation) {
				operation.Compensation["operation"] = "missing"
			},
			wantErr: `target "missing" is not defined`,
		},
		{
			name: "parameters are not a mapping",
			mutate: func(operation *Operation) {
				operation.Compensation["parameters"] = "id=1"
			},
			wantErr: "parameters must be a mapping",
		},
		{
			name: "mapping target is undeclared",
			mutate: func(operation *Operation) {
				operation.Compensation["parameters"] = map[string]interface{}{"host": "untrusted"}
			},
			wantErr: `parameter "host" is not declared`,
		},
		{
			name: "required target parameter cannot be produced",
			mutate: func(operation *Operation) {
				operation.Compensation["parameters"] = map[string]interface{}{}
			},
			wantErr: `requires parameter "reason"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := compensationSourceOperation("undo")
			source.Compensation["parameters"] = map[string]interface{}{"reason": "rollback"}
			tc.mutate(&source)
			target := irreversibleOperation("POST", "/undo")
			target.Params.BodySchema = bodySchemaWithRequired("reason")
			target.Body = map[string]interface{}{"reason": "{{ params.reason }}"}
			def := Definition{
				Version: "v1",
				Clients: map[string]Client{"api": {
					Operations: map[string]Operation{"mutate": source, "undo": target},
				}},
			}

			require.ErrorContains(t, ValidateDefinition(def), tc.wantErr)
		})
	}
}

func compensationSourceOperation(target string) Operation {
	return Operation{
		Method:  "POST",
		Path:    "/things",
		Success: StatusMapping{Status: []int{200}, Signal: "RESTResourceWritten"},
		SideEffects: []SideEffect{{
			Kind: "external_api", Target: "api.thing", State: "created",
		}},
		Reversibility: Reversibility{Classification: "compensatable", Undo: target},
		Compensation:  map[string]interface{}{"operation": target},
	}
}

func irreversibleOperation(method, path string) Operation {
	return Operation{
		Method:  method,
		Path:    path,
		Success: StatusMapping{Status: []int{200}, Signal: "RESTResourceWritten"},
		SideEffects: []SideEffect{{
			Kind: "external_api", Target: "api.thing", State: "mutated",
		}},
		Reversibility: Reversibility{
			Classification: "irreversible", Undo: "irreversible", RequiresConfirmation: true,
		},
	}
}
