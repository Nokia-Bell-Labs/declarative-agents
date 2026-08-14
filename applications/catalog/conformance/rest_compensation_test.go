// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"testing"
)

func TestRestPaymentCreateDeclaresExplicitCancelCompensation(t *testing.T) {
	var config struct {
		Rest struct {
			OpenAPI map[string]struct {
				Expose []string `yaml:"expose"`
			} `yaml:"openapi"`
			Clients map[string]struct {
				Operations map[string]struct {
					OpenAPIOperationID string `yaml:"openapi_operation_id"`
					Compensation       struct {
						Operation string `yaml:"operation"`
					} `yaml:"compensation"`
					Reversibility struct {
						Classification       string `yaml:"classification"`
						RequiresConfirmation bool   `yaml:"requires_confirmation"`
					} `yaml:"reversibility"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	unmarshalShipped(t, "testdata/conformance/rest/rest.yaml", &config)

	for _, operation := range config.Rest.OpenAPI["payments"].Expose {
		if operation == "createPayment" {
			t.Fatal("generated createPayment remains exposed without its own compensation")
		}
	}
	operations := config.Rest.Clients["payments"].Operations
	create := operations["create_payment"]
	if create.Reversibility.Classification != "compensatable" {
		t.Errorf("create payment REST policy = %q, want compensatable",
			create.Reversibility.Classification)
	}
	if got := create.Compensation.Operation; got != "cancel_payment" {
		t.Errorf("create payment compensation = %q, want cancel_payment", got)
	}
	cancel := operations["cancel_payment"]
	if cancel.OpenAPIOperationID != "cancelPayment" {
		t.Errorf("cancel operation ID = %q, want cancelPayment", cancel.OpenAPIOperationID)
	}
	if cancel.Reversibility.Classification != "irreversible" ||
		!cancel.Reversibility.RequiresConfirmation {
		t.Errorf("cancel payment policy = %+v, want confirmed irreversible", cancel.Reversibility)
	}

	var openapi struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
			Parameters  []struct {
				Name     string `yaml:"name"`
				In       string `yaml:"in"`
				Required bool   `yaml:"required"`
			} `yaml:"parameters"`
		} `yaml:"paths"`
	}
	unmarshalShipped(t, "testdata/conformance/rest/openapi/payments.yaml", &openapi)
	post := openapi.Paths["/payments/{id}/cancel"]["post"]
	if post.OperationID != "cancelPayment" || len(post.Parameters) != 1 ||
		post.Parameters[0].Name != "id" || post.Parameters[0].In != "path" ||
		!post.Parameters[0].Required {
		t.Errorf("cancel endpoint cannot consume the original required ID: %+v", post)
	}

	var declarations struct {
		Tools []struct {
			Name          string `yaml:"name"`
			Reversibility struct {
				Classification string `yaml:"classification"`
			} `yaml:"reversibility"`
		} `yaml:"tools"`
	}
	unmarshalShipped(t, "testdata/conformance/rest/declarations.yaml", &declarations)
	for _, tool := range declarations.Tools {
		if tool.Name == "create_payment" {
			if tool.Reversibility.Classification != "compensatable" {
				t.Errorf("create payment ToolDef policy = %q, want compensatable",
					tool.Reversibility.Classification)
			}
			return
		}
	}
	t.Fatal("create_payment ToolDef not found")
}
