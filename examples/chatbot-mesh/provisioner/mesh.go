// Copyright (c) 2026 Nokia. All rights reserved.

// Command provisioner is the chatbot-mesh deployment-plane API (srd003 R4): a
// minimal in-cluster service the provisioning panel drives to view the mesh and
// submit values edits that trigger a rollout. It edits deployment values and
// triggers rollouts only; it never submits an endpoint to a running agent
// (R4.2), the deployment-plane counterpart of agent-core srd028 V30. Authority is
// set by declared, validated config, never by this control surface.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// MeshView is the deployment-plane view of the mesh the panel reads and edits. It
// is values-plane only: RAG topology, the LLM endpoint, and the interesting
// parameters. It carries no per-agent runtime endpoint override, so a patch
// cannot smuggle transport authority to a running agent (R4.2).
type MeshView struct {
	Rags   []RagView  `json:"rags"`
	LLM    LLMView    `json:"llm"`
	Params ParamsView `json:"params"`
}

// RagView is one RAG unit. Status is read-only, populated from the monitor list on
// the read path; it is ignored on apply.
type RagView struct {
	Name           string `json:"name"`
	Collection     string `json:"collection"`
	EmbeddingModel string `json:"embeddingModel"`
	Replicas       int    `json:"replicas"`
	Status         string `json:"status,omitempty"`
}

// LLMView is the chat/embedding LLM endpoint: an external URL or the in-cluster
// Ollama tier (srd003 R6). When in-cluster, the model set -- the embedding model,
// the chat models, and the router model -- and the topology are the values the
// preload Job and the agents' config share. These are deployment values rendered
// into config, not submitted to a running agent.
type LLMView struct {
	InCluster   bool     `json:"inCluster"`
	ExternalURL string   `json:"externalURL"`
	EmbedModel  string   `json:"embedModel"`
	ChatModels  []string `json:"chatModels"`
	RouterModel string   `json:"routerModel"`
	Topology    string   `json:"topology,omitempty"`
	// ChatModel is the first chat model, kept for the panel's single-model summary.
	ChatModel string `json:"chatModel,omitempty"`
}

// ParamsView groups the interesting parameters (srd003 parameter grouping): the
// per-RAG retrieval bound, the composed-chunk cap, and the router default word.
type ParamsView struct {
	NResults      int    `json:"nResults"`
	ChunkCap      int    `json:"chunkCap"`
	RouterDefault string `json:"routerDefault"`
}

var dnsLabel = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)

// Validate rejects a patch that is not well-formed values-plane content, so a
// malformed or authority-bearing edit never reaches a rollout. It is the apply
// gate (R4.1) and, by admitting only these fields, the structural half of R4.2.
func (m MeshView) Validate() error {
	if len(m.Rags) == 0 {
		return fmt.Errorf("mesh must declare at least one RAG unit")
	}
	seen := map[string]bool{}
	for i, r := range m.Rags {
		if !dnsLabel.MatchString(r.Name) {
			return fmt.Errorf("rag[%d].name %q is not a DNS label", i, r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("rag[%d].name %q is duplicated", i, r.Name)
		}
		seen[r.Name] = true
		if strings.TrimSpace(r.Collection) == "" {
			return fmt.Errorf("rag[%d] %q: collection is required", i, r.Name)
		}
		if strings.TrimSpace(r.EmbeddingModel) == "" {
			return fmt.Errorf("rag[%d] %q: embeddingModel is required", i, r.Name)
		}
		if r.Replicas < 0 {
			return fmt.Errorf("rag[%d] %q: replicas must be >= 0", i, r.Name)
		}
	}
	if !m.LLM.InCluster && strings.TrimSpace(m.LLM.ExternalURL) == "" {
		return fmt.Errorf("llm: an external URL is required when not in-cluster")
	}
	if m.LLM.InCluster {
		if strings.TrimSpace(m.LLM.EmbedModel) == "" || strings.TrimSpace(m.LLM.RouterModel) == "" || len(m.LLM.ChatModels) == 0 {
			return fmt.Errorf("llm: the in-cluster tier requires an embedding model, a router model, and at least one chat model")
		}
		if m.LLM.Topology != "" && m.LLM.Topology != "single" && m.LLM.Topology != "per-model" {
			return fmt.Errorf("llm.topology %q must be single or per-model", m.LLM.Topology)
		}
	}
	if m.Params.NResults <= 0 {
		return fmt.Errorf("params.nResults must be > 0")
	}
	if m.Params.ChunkCap < 0 {
		return fmt.Errorf("params.chunkCap must be >= 0")
	}
	return nil
}

// HelmSetArgs renders the mesh view as helm --set arguments for the rollout, so
// the apply path re-renders the same co-generated topology (srd003 R2) the chart
// produces from these values. RAG status is read-only and never rendered.
func (m MeshView) HelmSetArgs() []string {
	var args []string
	for i, r := range m.Rags {
		args = append(args,
			fmt.Sprintf("ragUnits[%d].name=%s", i, r.Name),
			fmt.Sprintf("ragUnits[%d].collection=%s", i, r.Collection),
			fmt.Sprintf("ragUnits[%d].embeddingModel=%s", i, r.EmbeddingModel),
			fmt.Sprintf("ragUnits[%d].replicas=%d", i, r.Replicas),
		)
	}
	args = append(args,
		fmt.Sprintf("ollama.enabled=%t", m.LLM.InCluster),
		fmt.Sprintf("llm.externalURL=%s", m.LLM.ExternalURL),
		fmt.Sprintf("chatbot.embeddingModel=%s", m.LLM.EmbedModel),
	)
	if m.LLM.InCluster {
		// The models named once flow to both the preload Job and the agents' config
		// (srd003 R6.2), so a values-patch that changes the chat models re-renders
		// the tier and the config together.
		args = append(args,
			fmt.Sprintf("ollama.models.embedding=%s", m.LLM.EmbedModel),
			fmt.Sprintf("ollama.models.router=%s", m.LLM.RouterModel),
		)
		for i, model := range m.LLM.ChatModels {
			args = append(args, fmt.Sprintf("ollama.models.chat[%d]=%s", i, model))
		}
		if m.LLM.Topology != "" {
			args = append(args, fmt.Sprintf("ollama.topology=%s", m.LLM.Topology))
		}
	}
	return args
}
