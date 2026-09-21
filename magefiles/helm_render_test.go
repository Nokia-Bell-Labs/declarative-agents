// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationKindRendersHaveTypeMeta(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, application := range []string{"agent-architecture", "chatbot-mesh", "coding-agent"} {
		t.Run(application, func(t *testing.T) {
			chart := stageKindRenderChart(t, root, application)
			values := filepath.Join(chart, "ci", "kind-values.yaml")
			out, err := exec.Command("helm", "template", "validation", chart, "-f", values).CombinedOutput()
			if err != nil {
				t.Fatalf("helm template: %v\n%s", err, out)
			}
			if err := validateRenderedHelmDocuments(string(out)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func stageKindRenderChart(t *testing.T, root, application string) string {
	t.Helper()
	chart, cleanup, err := stageChartForRender(root, application)
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	t.Cleanup(cleanup)
	return chart
}

func TestValidateRenderedHelmDocumentsRejectsHeaderFailures(t *testing.T) {
	cases := map[string]string{
		"header concatenation":  "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-ClauseapiVersion: v1\nkind: ConfigMap\n",
		"comment-only document": "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\n",
	}
	for name, rendered := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateRenderedHelmDocuments(rendered); err == nil {
				t.Fatal("accepted malformed rendered document")
			}
		})
	}
}

func validateRenderedHelmDocuments(rendered string) error {
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	documents := strings.Split("\n"+rendered, "\n---\n")
	for index, document := range documents {
		document = strings.TrimSpace(document)
		if document == "" {
			continue
		}
		var missing []string
		for _, field := range []string{"apiVersion", "kind"} {
			if !renderedDocumentHasField(document, field) {
				missing = append(missing, field+":")
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("rendered document %d (%s) lacks standalone %s",
				index, renderedDocumentSource(document), strings.Join(missing, " and "))
		}
	}
	return nil
}

func renderedDocumentHasField(document, field string) bool {
	prefix := field + ":"
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, prefix) &&
			strings.TrimSpace(strings.TrimPrefix(line, prefix)) != "" {
			return true
		}
	}
	return false
}

func renderedDocumentSource(document string) string {
	for _, line := range strings.Split(document, "\n") {
		if source, ok := strings.CutPrefix(line, "# Source: "); ok {
			return source
		}
	}
	first, _, _ := strings.Cut(document, "\n")
	return first
}
