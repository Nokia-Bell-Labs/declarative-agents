// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Audit validates the application documentation, canonical profile boot
// closure, and every formal Go-test evidence claim.
func Audit() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := auditDocumentation(root); err != nil {
		return err
	}
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	manifest, err := readApplicationProfileManifest(filepath.Join(root, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		return err
	}
	packagedRoot, err := os.MkdirTemp("", "coding-agent-profiles-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(packagedRoot) }()
	packagedRoot = filepath.Join(packagedRoot, "profiles")
	source, err := inspectPackageSource(roots.Profiles, manifest.AgentProfiles.CompatibleRelease)
	if err != nil {
		return err
	}
	if _, err := assembleProfileClosure(manifest, roots.Profiles, packagedRoot, source); err != nil {
		return fmt.Errorf("assemble canonical profile closure: %w", err)
	}
	binary, cleanup, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanup()
	profiles := make([]string, 0, len(manifest.AgentProfiles.References))
	for _, ref := range manifest.AgentProfiles.References {
		profiles = append(profiles, filepath.Join(packagedRoot, filepath.FromSlash(ref.RuntimePath)))
	}
	if err := bootSmokeProfiles(binary, roots.Core, profiles); err != nil {
		return err
	}
	if err := validateTestEvidence(binary, root); err != nil {
		return err
	}
	return runTestEvidence(binary, root)
}

func auditDocumentation(root string) error {
	const validator = `
import pathlib
import sys
import yaml

root = pathlib.Path(sys.argv[1])
docs = root / "docs"
paths = sorted(docs.rglob("*.yaml"))
loaded = {path: yaml.safe_load(path.read_text()) for path in paths}
required = {
    docs / "VISION.yaml": {"id", "title", "executive_summary", "problem", "what_this_does", "why_we_build_this", "success_criteria", "not"},
    docs / "ARCHITECTURE.yaml": {"id", "title", "overview", "interfaces", "components", "design_decisions", "technology_choices", "project_structure", "implementation_status", "related_documents"},
    docs / "road-map.yaml": {"id", "title", "releases"},
    docs / "SPECIFICATIONS.yaml": {"id", "title", "overview", "roadmap_summary", "foundation_document_index", "srd_index", "config_format_index", "semantic_model_index", "use_case_index", "test_suite_index", "coverage_gaps"},
}
errors = []
for path, fields in required.items():
    missing = fields - set(loaded.get(path, {}))
    if missing:
        errors.append(f"{path.relative_to(root)} missing {sorted(missing)}")
index = loaded[docs / "SPECIFICATIONS.yaml"]
for section in ("foundation_document_index", "use_case_index", "test_suite_index"):
    for entry in index[section]:
        if not (root / entry["path"]).is_file():
            errors.append(f"{section} path does not exist: {entry['path']}")
use_case = loaded[docs / "specs/use-cases/rel01.0-uc001-coding-loop.yaml"]
suite = loaded[docs / "specs/test-suites/test-rel01.0-coding-loop.yaml"]
if use_case.get("test_suite") != suite.get("id"):
    errors.append("use case does not name the coding-loop test suite")
if use_case.get("id") not in suite.get("traces", []):
    errors.append("test suite does not trace the coding-loop use case")
if [case.get("stage") for case in suite.get("test_cases", [])] != ["A", "B", "C"]:
    errors.append("test suite must define ordered stages A, B, and C")
if errors:
    raise SystemExit("\n".join(errors))
print(f"audit: parsed and validated {len(paths)} coding-agent YAML documents")
`
	cmd := exec.Command("python3", "-c", validator, root)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("audit coding-agent docs: %w", err)
	}
	return nil
}

func bootSmokeProfiles(binary, coreRoot string, profiles []string) error {
	var failures []string
	for _, profile := range profiles {
		if err := runAgentPreflight(binary, "--validate-config", "--profile", profile, "--core-root", coreRoot); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", profile, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("canonical profile boot smoke failed:\n%s", strings.Join(failures, "\n"))
	}
	fmt.Printf("boot smoke passed for %d canonical coding profiles\n", len(profiles))
	return nil
}

func runAgentPreflight(binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}
