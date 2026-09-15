// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

type declarationEdge struct {
	kind     string
	importer declarationOwner
	imported declarationOwner
}

type declarationOwner struct {
	unit string
	path string
}

func validateImportUsedness(
	selected []catalog.ToolDef,
	rest toolrest.Collection,
	toolImports []catalog.ToolImport,
	typeUsed map[string]bool,
) error {
	toolUsed := selectedToolSources(selected)
	// A type unit contributes no tools, so its import earns its place through a
	// selected tool's schema reaching one of its types (srd051 R5.1).
	for path := range typeUsed {
		toolUsed[path] = true
	}
	restUsed := selectedRESTSources(selected, rest)
	edges := append(toolDeclarationEdges(toolImports), restDeclarationEdges(rest.DeclarationImports())...)
	var diagnostics []string
	diagnostics = append(diagnostics, unusedImportDiagnostics(edges, "tool", toolUsed)...)
	diagnostics = append(diagnostics, unusedImportDiagnostics(edges, "REST", restUsed)...)
	if len(diagnostics) == 0 {
		return nil
	}
	sort.Strings(diagnostics)
	return fmt.Errorf("unused declaration imports: %s", strings.Join(diagnostics, "; "))
}

func selectedToolSources(selected []catalog.ToolDef) map[string]bool {
	used := map[string]bool{}
	for _, tool := range selected {
		if source := tool.DeclarationSource(); source.Path != "" {
			used[source.Path] = true
		}
		if target := tool.OverrideTarget(); target.Path != "" {
			used[target.Path] = true
		}
	}
	return used
}

func selectedRESTSources(
	selected []catalog.ToolDef, rest toolrest.Collection,
) map[string]bool {
	clients, servers := restRoots(selected, rest)
	used := map[string]bool{}
	for name := range clients {
		markRESTClient(rest, name, used)
	}
	for name := range servers {
		markRESTServer(rest, name, used)
	}
	return used
}

func restRoots(
	selected []catalog.ToolDef, rest toolrest.Collection,
) (map[string]bool, map[string]bool) {
	clients, servers := map[string]bool{}, map[string]bool{}
	for _, tool := range selected {
		collectRESTRoots(tool.Config, clients, servers, rest)
	}
	return clients, servers
}

func collectRESTRoots(
	value any,
	clients, servers map[string]bool,
	rest toolrest.Collection,
) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if name, ok := child.(string); ok {
				addRESTRoot(key, name, clients, servers, rest)
			}
			collectRESTRoots(child, clients, servers, rest)
		}
	case []interface{}:
		for _, child := range typed {
			collectRESTRoots(child, clients, servers, rest)
		}
	}
}

func addRESTRoot(
	key, name string,
	clients, servers map[string]bool,
	rest toolrest.Collection,
) {
	switch key {
	case "rest_ref":
		if _, ok := rest.Clients[name]; ok {
			clients[name] = true
		}
		if _, ok := rest.Servers[name]; ok {
			servers[name] = true
		}
	case "client", "client_ref":
		clients[name] = true
	case "server", "server_ref":
		servers[name] = true
	}
}

func markRESTClient(rest toolrest.Collection, name string, used map[string]bool) {
	client, ok := rest.Clients[name]
	if !ok {
		return
	}
	markRESTEntry(rest, "clients", name, used)
	markRESTEntry(rest, "auth", client.AuthRef, used)
	markRESTEntry(rest, "limits", client.LimitsRef, used)
	markRESTEntry(rest, "retry_policies", client.RetryRef, used)
	markOpenAPISources(rest, "client", name, used)
	for _, operation := range client.Operations {
		markRESTOperation(rest, operation, used)
	}
	for _, resource := range client.Resources {
		for _, operation := range resource.Operations {
			markRESTOperation(rest, operation, used)
		}
	}
}

func markRESTOperation(
	rest toolrest.Collection,
	operation restdef.Operation,
	used map[string]bool,
) {
	markRESTEntry(rest, "response_mappings", operation.ResponseRef, used)
	markRESTEntry(rest, "response_mappings", operation.Success.ResponseRef, used)
	for _, failure := range operation.Failures {
		markRESTEntry(rest, "response_mappings", failure.ResponseRef, used)
	}
}

func markRESTServer(rest toolrest.Collection, name string, used map[string]bool) {
	server, ok := rest.Servers[name]
	if !ok {
		return
	}
	markRESTEntry(rest, "servers", name, used)
	markRESTEntry(rest, "limits", server.LimitsRef, used)
	markRESTEntry(rest, "auth", server.LifecycleExit.AuthRef, used)
	markOpenAPISources(rest, "server", name, used)
	for _, endpoint := range server.Endpoints {
		markRESTEntry(rest, "auth", endpoint.LifecycleControl.RequireAuthRef, used)
		for _, resource := range endpoint.MachineRequest.DocumentResources {
			markRESTEntry(rest, "document_resources", resource, used)
		}
	}
}

func markOpenAPISources(
	rest toolrest.Collection,
	kind, name string,
	used map[string]bool,
) {
	for _, source := range rest.OpenAPISourcesFor(kind, name) {
		used[source.Path] = true
	}
}

func markRESTEntry(
	rest toolrest.Collection,
	family, name string,
	used map[string]bool,
) {
	if name == "" {
		return
	}
	if source, ok := rest.DeclarationSource(family, name); ok {
		used[source.Path] = true
	}
}

func toolDeclarationEdges(imports []catalog.ToolImport) []declarationEdge {
	edges := make([]declarationEdge, len(imports))
	for index, edge := range imports {
		edges[index] = declarationEdge{
			kind:     "tool",
			importer: declarationOwner{unit: edge.Importer.Unit, path: edge.Importer.Path},
			imported: declarationOwner{unit: edge.Imported.Unit, path: edge.Imported.Path},
		}
	}
	return edges
}

func restDeclarationEdges(imports []restdef.DeclarationImport) []declarationEdge {
	edges := make([]declarationEdge, len(imports))
	for index, edge := range imports {
		edges[index] = declarationEdge{
			kind:     "REST",
			importer: declarationOwner{unit: edge.Importer.Unit, path: edge.Importer.Path},
			imported: declarationOwner{unit: edge.Imported.Unit, path: edge.Imported.Path},
		}
	}
	return edges
}

func unusedImportDiagnostics(
	edges []declarationEdge, kind string, used map[string]bool,
) []string {
	propagateImportUsedness(edges, kind, used)
	var diagnostics []string
	for _, edge := range edges {
		if edge.kind == kind && !used[edge.imported.path] {
			diagnostics = append(diagnostics, fmt.Sprintf(
				"%s unit %q at %s imported by unit %q at %s",
				kind, edge.imported.unit, edge.imported.path,
				edge.importer.unit, edge.importer.path,
			))
		}
	}
	return diagnostics
}

func propagateImportUsedness(edges []declarationEdge, kind string, used map[string]bool) {
	for changed := true; changed; {
		changed = false
		for _, edge := range edges {
			if edge.kind != kind || !used[edge.imported.path] || used[edge.importer.path] {
				continue
			}
			used[edge.importer.path] = true
			changed = true
		}
	}
}
