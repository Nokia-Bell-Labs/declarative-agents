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
// the reference: the unit, or the fragment an instantiation came from.
func (r *toolImportResolver) resolveConfigFiles(defs []ToolDef, declaring string) error {
	if r.options.KeepConfigFiles {
		return nil
	}
	for index := range defs {
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
