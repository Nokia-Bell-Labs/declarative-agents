// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

type documentationCompensationOperation struct {
	Path   string `yaml:"path"`
	Params struct {
		Path       map[string]interface{} `yaml:"path"`
		BodySchema struct {
			Required []string `yaml:"required"`
		} `yaml:"body_schema"`
	} `yaml:"params"`
	Compensation struct {
		Operation  string                 `yaml:"operation"`
		Parameters map[string]interface{} `yaml:"parameters"`
	} `yaml:"compensation"`
	Reversibility struct {
		Classification       string `yaml:"classification"`
		RequiresConfirmation bool   `yaml:"requires_confirmation"`
	} `yaml:"reversibility"`
}

func TestDocumentationSuggestionDeclaresExecutableCompensation(t *testing.T) {
	var config struct {
		Rest struct {
			ResponseMappings map[string]struct {
				ResourceID string `yaml:"resource_id"`
			} `yaml:"response_mappings"`
			Clients map[string]struct {
				Operations map[string]documentationCompensationOperation `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	readDocumentationCuratorYAML(t, "rest.yaml", &config)

	if got := config.Rest.ResponseMappings["doc_suggestions"].ResourceID; got != "$.patch_id" {
		t.Errorf("suggestion resource ID = %q, want $.patch_id", got)
	}
	operations := config.Rest.Clients["documentation"].Operations
	source := operations["doc_suggest_changes"]
	if source.Reversibility.Classification != "compensatable" {
		t.Errorf("suggestion REST policy = %q, want compensatable",
			source.Reversibility.Classification)
	}
	if source.Compensation.Operation != "doc_suggestion_reject" {
		t.Errorf("suggestion compensation = %q, want doc_suggestion_reject", source.Compensation.Operation)
	}
	if got := source.Compensation.Parameters["decided_by"]; got != "rollback" {
		t.Errorf("rollback actor = %v, want rollback", got)
	}

	target := operations["doc_suggestion_reject"]
	if target.Path != "/api/v1/docs/patches/{id}/reject" {
		t.Errorf("suggestion rejection path = %q", target.Path)
	}
	if _, ok := target.Params.Path["id"]; !ok {
		t.Error("suggestion rejection does not accept the receipt resource ID alias")
	}
	if len(target.Params.BodySchema.Required) != 1 || target.Params.BodySchema.Required[0] != "decided_by" {
		t.Errorf("suggestion rejection required body fields = %v, want [decided_by]",
			target.Params.BodySchema.Required)
	}
	if target.Reversibility.Classification != "irreversible" ||
		!target.Reversibility.RequiresConfirmation {
		t.Errorf("suggestion rejection policy = %+v, want confirmed irreversible",
			target.Reversibility)
	}

	var declarations struct {
		Tools []struct {
			Name          string `yaml:"name"`
			Reversibility struct {
				Classification string `yaml:"classification"`
			} `yaml:"reversibility"`
		} `yaml:"tools"`
	}
	readDocumentationCuratorYAML(t, "declarations.yaml", &declarations)
	for _, tool := range declarations.Tools {
		if tool.Name == "doc_suggest_changes" {
			if tool.Reversibility.Classification != "compensatable" {
				t.Errorf("suggestion ToolDef policy = %q, want compensatable",
					tool.Reversibility.Classification)
			}
			return
		}
	}
	t.Fatal("doc_suggest_changes ToolDef not found")
}
