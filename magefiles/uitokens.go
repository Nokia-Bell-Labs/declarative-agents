// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// uiKitTokensSpecifier is the only import a UI may use for the shared design
// tokens (srd004 R8.2, srd003 R5.3).
const uiKitTokensSpecifier = uiKitPackageName + "/tokens.css"

// resolveUITokenImport checks that a UI's App.css imports the kit tokens first
// and redeclares none of them, then resolves the import through the UI's
// file: dependency on the kit and the kit's exports map. It returns the token
// file the import reaches; callers compare it with canonicalUITokensPath.
func resolveUITokenImport(cssPath string, css []byte) (string, error) {
	first := strings.SplitN(string(css), "\n", 2)[0]
	if first != `@import "`+uiKitTokensSpecifier+`";` {
		return "", fmt.Errorf("%s: first line must be @import %q, got %q", cssPath, uiKitTokensSpecifier, first)
	}
	if strings.Contains(string(css), "--bg-primary:") {
		return "", fmt.Errorf("%s: redeclares canonical design-token values", cssPath)
	}
	uiDir := filepath.Dir(filepath.Dir(cssPath))
	var ui struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := readJSONFile(filepath.Join(uiDir, "package.json"), &ui); err != nil {
		return "", err
	}
	spec := ui.Dependencies[uiKitPackageName]
	if !strings.HasPrefix(spec, "file:") {
		return "", fmt.Errorf("%s: dependency %s = %q, want a file: path to %s", uiDir, uiKitPackageName, spec, uiKitDir)
	}
	kitDir := filepath.Clean(filepath.Join(uiDir, filepath.FromSlash(strings.TrimPrefix(spec, "file:"))))
	var kit struct {
		Exports map[string]json.RawMessage `json:"exports"`
	}
	if err := readJSONFile(filepath.Join(kitDir, "package.json"), &kit); err != nil {
		return "", err
	}
	var target string
	if err := json.Unmarshal(kit.Exports["./tokens.css"], &target); err != nil || target == "" {
		return "", fmt.Errorf("%s: package.json exports no ./tokens.css file", kitDir)
	}
	return filepath.Clean(filepath.Join(kitDir, filepath.FromSlash(target))), nil
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
