// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"fmt"
	"slices"
	"strings"
)

// applierComponent is the component label the shared library puts on the
// applier's objects, its Role among them.
const applierComponent = "applier"

// checkApplierCoversRenderedKinds applies R11.1: an applier whose Role may
// write re-renders the whole chart on every upgrade, so its Role must let it
// read and write every object kind the chart renders. A kind it cannot read
// fails the upgrade with a 403 on the first object of that kind, which only a
// live cluster otherwise shows (GH-2533). A read-only applier (no write verb
// in its Role) never upgrades and is not checked.
func checkApplierCoversRenderedKinds(chart, overlay string, documents []Document) []Finding {
	var findings []Finding
	for _, role := range documents {
		if role.Kind != "Role" || role.Metadata.Labels["app.kubernetes.io/component"] != applierComponent || !grantsAnyWrite(role.Rules) {
			continue
		}
		seen := map[string]bool{}
		for _, document := range documents {
			group, resource := groupResource(document)
			key := qualifiedResource(group, resource)
			if seen[key] {
				continue
			}
			seen[key] = true
			if grants(role.Rules, group, resource, "get") && (grants(role.Rules, group, resource, "update") || grants(role.Rules, group, resource, "patch")) {
				continue
			}
			findings = append(findings, Finding{
				Chart: chart, Overlay: overlay, Rule: "R11.1",
				Resource: role.Resource(), Value: key,
				Detail: fmt.Sprintf("the chart renders %s but the applier Role cannot read and write it, so an in-cluster upgrade is refused", document.Resource()),
			})
		}
	}
	return findings
}

func grantsAnyWrite(rules []PolicyRule) bool {
	for _, rule := range rules {
		for _, verb := range rule.Verbs {
			if verb == "*" || verb == "create" || verb == "update" || verb == "patch" {
				return true
			}
		}
	}
	return false
}

func grants(rules []PolicyRule, group, resource, verb string) bool {
	for _, rule := range rules {
		if matches(rule.APIGroups, group) && matches(rule.Resources, resource) && matches(rule.Verbs, verb) {
			return true
		}
	}
	return false
}

func matches(values []string, want string) bool {
	return slices.Contains(values, want) || slices.Contains(values, "*")
}

// groupResource maps a rendered object to the RBAC API group and resource
// name a Role rule grants it by.
func groupResource(document Document) (string, string) {
	group := ""
	if slash := strings.Index(document.APIVersion, "/"); slash >= 0 {
		group = document.APIVersion[:slash]
	}
	return group, pluralResource(document.Kind)
}

// pluralResource lowercases and pluralizes a kind the way the Kubernetes API
// names its resources for every kind a chart here renders.
func pluralResource(kind string) string {
	lower := strings.ToLower(kind)
	switch {
	case strings.HasSuffix(lower, "ss"), strings.HasSuffix(lower, "ch"), strings.HasSuffix(lower, "sh"), strings.HasSuffix(lower, "x"):
		return lower + "es"
	case strings.HasSuffix(lower, "y") && !strings.HasSuffix(lower, "ey"):
		return strings.TrimSuffix(lower, "y") + "ies"
	default:
		return lower + "s"
	}
}

func qualifiedResource(group, resource string) string {
	if group == "" {
		return resource
	}
	return group + "/" + resource
}
