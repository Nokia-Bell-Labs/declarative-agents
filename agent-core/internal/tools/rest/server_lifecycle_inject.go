// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

import "net/http"

// injectLifecycleExit synthesizes the canonical POST /api/lifecycle/exit
// lifecycle_control endpoint so every served agent exposes lifecycle control
// from agent-core rather than each profile re-declaring it (GH-1263, GH-1264).
// Injection is skipped when the server opts out, already declares the canonical
// path, or already uses the reserved route name, so injection fills the gap
// without overriding a profile's explicit intent. The declared endpoint map is
// never mutated: a fresh map is returned only when an endpoint is added, since
// the source map is shared with the resolving Collection.
func injectLifecycleExit(server restdef.Server) map[string]restdef.Endpoint {
	if server.LifecycleExit.Disabled || lifecycleExitDeclared(server.Endpoints) {
		return server.Endpoints
	}
	injected := make(map[string]restdef.Endpoint, len(server.Endpoints)+1)
	for name, endpoint := range server.Endpoints {
		injected[name] = endpoint
	}
	injected[lifecycleExitRouteName] = canonicalLifecycleExitEndpoint(server.LifecycleExit.AuthRef)
	return injected
}

// lifecycleExitDeclared reports whether a server already carries a lifecycle
// exit surface, either under the reserved route name or at the canonical path,
// so injection neither overwrites a map entry nor conflicts on the route.
func lifecycleExitDeclared(endpoints map[string]restdef.Endpoint) bool {
	if _, ok := endpoints[lifecycleExitRouteName]; ok {
		return true
	}
	for _, endpoint := range endpoints {
		if endpoint.Method == http.MethodPost && endpoint.Path == lifecycleExitPath {
			return true
		}
	}
	return false
}

// canonicalLifecycleExitEndpoint is the injected exit route: a lifecycle_control
// endpoint that enqueues ExitRequested for MachineSpec routing. AuthRef is empty
// for parity with today's auth:none control servers.
func canonicalLifecycleExitEndpoint(authRef string) restdef.Endpoint {
	return restdef.Endpoint{
		Method:  http.MethodPost,
		Path:    lifecycleExitPath,
		Binding: bindingLifecycleControl,
		Signal:  lifecycleExitSignal,
		LifecycleControl: restdef.LifecycleControl{
			Action:         lifecycleActionEnqueueSignal,
			Signal:         lifecycleExitSignal,
			RequireAuthRef: authRef,
		},
		Request: restdef.RequestBinding{BodySchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{"type": "string"},
				"status": map[string]interface{}{"type": "string"},
			},
		}},
		Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
	}
}
