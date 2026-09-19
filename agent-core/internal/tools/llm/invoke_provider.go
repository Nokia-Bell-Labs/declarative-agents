// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"fmt"
	"net/http"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
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
// dialect. A config that still names provider resolves to the shipped
// library's dialect, so a declaration written before srd058 keeps working
// while it migrates (R3.2).
func resolveProvider(cfg catalog.LLMToolConfig, deps InvokeLLMFactoryDeps) (resolvedProvider, error) {
	if cfg.Dialect == "" {
		path, err := legacyDialect(cfg.Provider)
		if err != nil {
			return resolvedProvider{}, err
		}
		cfg.Dialect = path
	}
	if cfg.ProviderURL == "" {
		return resolvedProvider{}, fmt.Errorf("invoke_llm config %s requires provider_url", dialectLabel(cfg))
	}
	return resolveDialect(cfg, deps)
}

// shippedProviders are the libraries agent-core installs; a legacy provider
// value names one of them.
var shippedProviders = map[string]bool{"ollama": true, "cohere": true}

// legacyDialect is the shipped chat dialect a provider value names, under the
// agent-core install root when one is set and where the runtime image
// installs it otherwise.
func legacyDialect(provider string) (string, error) {
	if !shippedProviders[provider] {
		return "", fmt.Errorf("unsupported invoke_llm provider %q; name a dialect instead", provider)
	}
	path := corepath.InstallPrefix + "/tools/providers/" + provider + "/" + dialect.FileName
	if mapped := corepath.Map(path); mapped != "" {
		return mapped, nil
	}
	return path, nil
}

func dialectLabel(cfg catalog.LLMToolConfig) string {
	if cfg.Provider != "" {
		return fmt.Sprintf("provider %q", cfg.Provider)
	}
	return fmt.Sprintf("dialect %q", cfg.Dialect)
}

func resolveDialect(cfg catalog.LLMToolConfig, deps InvokeLLMFactoryDeps) (resolvedProvider, error) {
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
