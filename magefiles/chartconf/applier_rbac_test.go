// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"testing"
)

// applierRender renders an Ingress and a Deployment beside an applier Role
// carrying the given rules, the shape the shared library emits.
func applierRender(rules string) string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: app-applier
  labels:
    app.kubernetes.io/component: applier
rules:
` + rules + `---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
`
}

const applierBaseRules = `- apiGroups: [apps]
  resources: [deployments]
  verbs: [get, list, watch, create, update, patch]
- apiGroups: [rbac.authorization.k8s.io]
  resources: [roles]
  verbs: [get, list, watch, create, update, patch]
`

func applierFindings(t *testing.T, rules string) []Finding {
	t.Helper()
	documents, err := Parse(applierRender(rules))
	if err != nil {
		t.Fatal(err)
	}
	return checkApplierCoversRenderedKinds("app", "defaults", documents)
}

func TestApplierRoleMissingARenderedKindIsReported(t *testing.T) {
	findings := applierFindings(t, applierBaseRules+`- apiGroups: [networking.k8s.io]
  resources: [networkpolicies]
  verbs: [get, list, watch, create, update, patch]
`)

	if len(findings) != 1 {
		t.Fatalf("findings = %v, want one for the Ingress", findings)
	}
	got := findings[0]
	if got.Rule != "R11.1" || got.Value != "networking.k8s.io/ingresses" || got.Resource != "Role/app-applier" {
		t.Fatalf("finding = %+v, want R11.1 naming networking.k8s.io/ingresses on Role/app-applier", got)
	}
}

func TestApplierRoleCoveringEveryRenderedKindPasses(t *testing.T) {
	findings := applierFindings(t, applierBaseRules+`- apiGroups: [networking.k8s.io]
  resources: [networkpolicies, ingresses]
  verbs: [get, list, watch, create, update, patch]
`)

	if len(findings) != 0 {
		t.Fatalf("findings = %v, want none", findings)
	}
}

func TestApplierRoleThatCanWriteButNotReadIsReported(t *testing.T) {
	findings := applierFindings(t, applierBaseRules+`- apiGroups: [networking.k8s.io]
  resources: [ingresses]
  verbs: [create, update, patch]
`)

	if len(findings) != 1 || findings[0].Value != "networking.k8s.io/ingresses" {
		t.Fatalf("findings = %v, want one: helm reads before it writes", findings)
	}
}

func TestReadOnlyApplierIsNotChecked(t *testing.T) {
	findings := applierFindings(t, `- apiGroups: [apps]
  resources: [deployments]
  verbs: [get, list, watch]
`)

	if len(findings) != 0 {
		t.Fatalf("findings = %v, want none: a read-only applier never upgrades", findings)
	}
}

func TestPluralResourceNamesTheKindsChartsRender(t *testing.T) {
	for kind, want := range map[string]string{
		"Ingress":               "ingresses",
		"NetworkPolicy":         "networkpolicies",
		"PersistentVolumeClaim": "persistentvolumeclaims",
		"Job":                   "jobs",
		"StatefulSet":           "statefulsets",
		"ConfigMap":             "configmaps",
		"ServiceAccount":        "serviceaccounts",
	} {
		if got := pluralResource(kind); got != want {
			t.Errorf("pluralResource(%q) = %q, want %q", kind, got, want)
		}
	}
}
