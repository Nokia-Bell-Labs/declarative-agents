// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// configFileFields are the ToolDef config fields that name a declaration file
// the tool reads when it is built: invoke_llm's chat dialect (srd058 R2.3).
// Each resolves like an import, relative to the declaring file or under a
// library root, and joins the closure, so the program identity and the dump
// name the file the tool ran with and staging carries it.
var configFileFields = []string{"dialect"}

// resolveConfigFiles rewrites each config file reference in defs to the path
// it resolves to and visits the file. declaring is the file whose text holds
// the reference: the unit, or the fragment an expansion came from.
func (r *toolImportResolver) resolveConfigFiles(defs []ToolDef, declaring string) error {
	if r.options.KeepConfigFiles {
		return nil
	}
	for index := range defs {
		r.adoptLegacyProviderDialect(&defs[index])
		for _, field := range configFileFields {
			written, ok := defs[index].Config[field].(string)
			if !ok || strings.TrimSpace(written) == "" {
				continue
			}
			target, err := r.readConfigFile(declaring, written)
			if err != nil {
				return fmt.Errorf("tool %q config %s %q: %w", defs[index].Name, field, written, err)
			}
			config := make(map[string]interface{}, len(defs[index].Config))
			for key, value := range defs[index].Config {
				config[key] = value
			}
			config[field] = target
			defs[index].Config = config
		}
	}
	return nil
}

func (r *toolImportResolver) readConfigFile(declaring, written string) (string, error) {
	target, err := corepath.ImportTarget(declaring, written)
	if errors.Is(err, corepath.ErrOutsideLibraryRoot) {
		return "", fmt.Errorf("an absolute path must sit under a library root")
	}
	if err != nil {
		return "", err
	}
	target = filepath.Clean(target)
	data, err := os.ReadFile(target)
	if err != nil {
		return "", missingConfigFile(written, target, err)
	}
	if r.visit != nil {
		if err := r.visit(target, data); err != nil {
			return "", err
		}
	}
	return target, nil
}

// missingConfigFile names the path as written, where it resolved, and, for a
// rooted path, the directory its root is bound to, so an operator who rebound
// a library sees which binding lacks the file (srd058 R1.3).
func missingConfigFile(written, target string, cause error) error {
	if !errors.Is(cause, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", target, cause)
	}
	root, _, rooted := strings.Cut(strings.TrimPrefix(filepath.ToSlash(written), corepath.LibraryPrefix+"/"), "/")
	if !filepath.IsAbs(written) || !rooted {
		return fmt.Errorf("%s does not exist", target)
	}
	if directory, bound := corepath.LibraryRoots()[root]; bound {
		return fmt.Errorf("%s does not exist: library root %q is bound to %s", target, root, directory)
	}
	return fmt.Errorf("%s does not exist under library root %q", target, root)
}

// legacyProviders are the provider values written before srd058; each names a
// library agent-core ships.
var legacyProviders = map[string]bool{"ollama": true, "cohere": true}

// adoptLegacyProviderDialect rewrites an invoke_llm config that names a legacy
// provider, or neither field, to the shipped library's dialect, so the dialect
// the tool runs with is a closure file like any other (srd058 R2.3, R3.2). A
// config left untouched (an unknown provider, or a shipped dialect that is not
// installed here) keeps its provider for the runtime to resolve or report.
func (r *toolImportResolver) adoptLegacyProviderDialect(def *ToolDef) {
	if def.Init != "invoke_llm" {
		return
	}
	if dialect, _ := def.Config["dialect"].(string); dialect != "" {
		return
	}
	provider, _ := def.Config["provider"].(string)
	if provider == "" {
		provider = "ollama"
	}
	if !legacyProviders[provider] {
		return
	}
	written := corepath.InstallPrefix + "/tools/providers/" + provider + "/chat-dialect.yaml"
	target := written
	if mapped := corepath.Map(written); mapped != "" {
		target = mapped
	}
	if _, err := os.Stat(target); err != nil {
		return
	}
	config := make(map[string]interface{}, len(def.Config)+1)
	for key, value := range def.Config {
		if key != "provider" {
			config[key] = value
		}
	}
	config["dialect"] = written
	// A legacy Ollama config without an endpoint meant the local default,
	// which invoke_llm keys to the provider value this rewrite removes.
	_, hasURL := config["provider_url"]
	_, hasLegacyURL := config["ollama_url"]
	if provider == "ollama" && !hasURL && !hasLegacyURL {
		config["provider_url"] = "http://localhost:11434"
	}
	def.Config = config
}
