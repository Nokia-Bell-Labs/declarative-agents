// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"fmt"
	"net/http"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm/cohere"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm/ollama"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm/dialect"
)

// resolvedProvider is what the provider-specific steps resolve to: the client
// that performs the call, the name spans carry, and the reply parser.
type resolvedProvider struct {
	client     modelllm.Client
	name       string
	serverAddr string
	parser     modelllm.ResponseParser
	profiles   *modelllm.ProfileRegistry
}

// resolveProvider fills invoke_llm's provider holes from the bound chat
// dialect, or, for a config that still names provider, from the compiled
// adapter it names.
func resolveProvider(cfg catalog.LLMToolConfig, deps InvokeLLMFactoryDeps) (resolvedProvider, error) {
	if cfg.Dialect != "" {
		return resolveDialect(cfg, deps)
	}
	registry, err := modelllm.DefaultProfileRegistry()
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("load profiles: %w", err)
	}
	parser, err := resolveParser(registry, cfg.ResponseProfile, "", cfg.Model)
	if err != nil {
		return resolvedProvider{}, err
	}
	client, addr, err := newLLMClient(cfg, deps.Tracer)
	if err != nil {
		return resolvedProvider{}, err
	}
	return resolvedProvider{client: client, name: cfg.Provider, serverAddr: addr, parser: parser, profiles: registry}, nil
}

func resolveDialect(cfg catalog.LLMToolConfig, deps InvokeLLMFactoryDeps) (resolvedProvider, error) {
	if cfg.ProviderURL == "" {
		return resolvedProvider{}, fmt.Errorf("invoke_llm config dialect %q requires provider_url", cfg.Dialect)
	}
	chat, err := dialect.Load(cfg.Dialect)
	if err != nil {
		return resolvedProvider{}, err
	}
	registry, err := chat.Profiles()
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("chat dialect %s: %w", cfg.Dialect, err)
	}
	parser, err := resolveParser(registry, cfg.ResponseProfile, chat.ParserProfile, cfg.Model)
	if err != nil {
		return resolvedProvider{}, err
	}
	client := newDialectClient(chat, cfg.ProviderURL, &http.Client{Timeout: httpTimeout(cfg)}, deps.Tracer, deps.Credentials)
	return resolvedProvider{
		client: client, name: chat.ProviderName, serverAddr: serverAddr(cfg.ProviderURL),
		parser: parser, profiles: registry,
	}, nil
}

func newLLMClient(cfg catalog.LLMToolConfig, tracer tracing.Tracer) (modelllm.Client, string, error) {
	if cfg.Provider != "ollama" && cfg.Provider != "cohere" {
		return nil, "", fmt.Errorf("unsupported invoke_llm provider %q", cfg.Provider)
	}
	if cfg.ProviderURL == "" {
		return nil, "", fmt.Errorf("invoke_llm config provider %q requires provider_url", cfg.Provider)
	}
	// Profiles that need preflight readiness declare a REST transition; adapter
	// construction performs no hidden network probe.
	var client modelllm.Client
	var err error
	switch cfg.Provider {
	case "ollama":
		client, err = ollama.NewAdapter(cfg.ProviderURL, cfg.Model,
			ollama.WithHTTPClient(&http.Client{Timeout: httpTimeout(cfg)}),
			ollama.WithTracer(tracerOrNoop(tracer)),
		)
	case "cohere":
		client, err = cohere.NewAdapter(cfg.ProviderURL, cfg.Model,
			cohere.WithHTTPClient(&http.Client{Timeout: httpTimeout(cfg)}),
			cohere.WithTracer(tracerOrNoop(tracer)),
		)
	}
	return client, serverAddr(cfg.ProviderURL), err
}

// resolveParser picks the reply parser: the config's response_profile, then
// the dialect's parser_profile, then the profile whose prefix matches the
// model. registry already holds a bound library's profiles ahead of the
// embedded ones.
func resolveParser(registry *modelllm.ProfileRegistry, configured, dialectDefault, model string) (modelllm.ResponseParser, error) {
	name := configured
	if name == "" {
		name = dialectDefault
	}
	if name == "" {
		return registry.ResolveProfile(model), nil
	}
	parser, ok := registry.ResolveProfileName(name)
	if !ok {
		return nil, fmt.Errorf("invoke_llm response_profile %q not found", name)
	}
	return parser, nil
}

func resolveLLMParser(cfg catalog.LLMToolConfig) (modelllm.ResponseParser, error) {
	registry, err := modelllm.DefaultProfileRegistry()
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	return resolveParser(registry, cfg.ResponseProfile, "", cfg.Model)
}
