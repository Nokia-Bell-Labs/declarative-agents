// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

// ClientResponse reports what a REST client word's configured operation
// declares about its response, so the label that word publishes can be typed
// from the same declaration the runtime reads (GH-2064).
//
// The mapped keys are united over the operation's status mappings. Each status
// may resolve its own response mapping, the status decides which applies, and
// the label is published whichever one fired, so a key any of them publishes
// can legitimately resolve and typing from success alone would reject a
// selector valid on a failure path.
func (c Collection) ClientResponse(
	config map[string]interface{},
) (catalog.RESTClientResponse, bool) {
	cfg := clientToolConfigFrom(config)
	if cfg.RestRef == "" || cfg.Operation == "" {
		return catalog.RESTClientResponse{}, false
	}
	def, err := c.ResolveClientOperation(cfg)
	if err != nil {
		return catalog.RESTClientResponse{}, false
	}
	return catalog.RESTClientResponse{
		Mapped:  c.mappedKeys(def),
		Carried: def.Operation.Params.CarryForward,
	}, true
}

// mappedKeys unites the response mapping output keys of every status the
// operation declares, and of the operation itself when it declares no status
// mapping of its own.
func (c Collection) mappedKeys(def restclient.ClientOperationDefinition) []string {
	statuses := append([]restdef.StatusMapping{def.Operation.Success}, def.Operation.Failures...)
	seen := map[string]bool{}
	var keys []string
	for _, status := range statuses {
		for name := range restclient.ResolvedResponseMapping(def, status).Output {
			if seen[name] {
				continue
			}
			seen[name] = true
			keys = append(keys, name)
		}
	}
	return keys
}

func clientToolConfigFrom(config map[string]interface{}) ClientToolConfig {
	return ClientToolConfig{
		RestRef:   configText(config, "rest_ref"),
		Resource:  configText(config, "resource"),
		Operation: configText(config, "operation"),
	}
}

func configText(config map[string]interface{}, key string) string {
	text, _ := config[key].(string)
	return text
}
