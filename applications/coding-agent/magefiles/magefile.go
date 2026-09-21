// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	source, err := inspectPackageSource(roots.Profiles, manifest.Catalog.CompatibleRelease)
	if err != nil {
		return err
	}
	if _, err := assembleProfileClosure(manifest, roots.Profiles, packagedRoot, source); err != nil {
		return fmt.Errorf("assemble canonical profile closure: %w", err)
	}
	deploymentRoot := filepath.Join(filepath.Dir(packagedRoot), "deployment")
	shards, err := packageServingDeployment(root, roots.Profiles, deploymentRoot, manifest, source)
	if err != nil {
		return fmt.Errorf("package serving deployment: %w", err)
	}
	binary, cleanup, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanup()
	profiles := make([]string, 0, len(manifest.Catalog.References))
	for _, ref := range manifest.Catalog.References {
		profiles = append(profiles, filepath.Join(packagedRoot, filepath.FromSlash(ref.RuntimePath)))
	}
	for _, shard := range shards {
		profiles = append(profiles,
			filepath.Join(deploymentRoot, shard.Path, filepath.FromSlash(shard.Profile)))
	}
	if err := bootSmokeProfiles(binary, roots.Core, profiles); err != nil {
		return err
	}
	if err := runCorpusAudit(binary, root, roots.Core, roots.Profiles); err != nil {
		return err
	}
	return runTestEvidenceAudit(binary, root, roots.Core, roots.Profiles)
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
for section in ("foundation_document_index", "srd_index", "use_case_index", "test_suite_index"):
    for entry in index[section]:
        if not (root / entry["path"]).is_file():
            errors.append(f"{section} path does not exist: {entry['path']}")
suite_by_id = {
    entry["id"]: loaded[root / entry["path"]]
    for entry in index["test_suite_index"]
}
for entry in index["use_case_index"]:
    use_case_doc = loaded[root / entry["path"]]
    suite_id = use_case_doc.get("test_suite")
    suite_doc = suite_by_id.get(suite_id)
    if suite_doc is None:
        errors.append(f"use case {entry['id']} names unknown test suite {suite_id}")
    elif entry["id"] not in suite_doc.get("traces", []):
        errors.append(f"test suite {suite_id} does not trace use case {entry['id']}")
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
	cmd, err := repositoryPython(root, "-c", validator, root)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("audit coding-agent docs: %w", err)
	}
	return nil
}

// repositoryPython runs the interpreter pixi resolves from the repository's
// pixi.toml, never the ambient python3. The audit needs PyYAML, and whichever
// interpreter happens to lead a developer's PATH may not carry it: installing
// an unrelated tool once put a Homebrew python ahead of the system one and
// failed this gate with a bare import traceback (GH-2446). A declared,
// locked environment is the repository's rule for Python.
func repositoryPython(applicationRoot string, args ...string) (*exec.Cmd, error) {
	manifest := filepath.Join(applicationRoot, "..", "..", "pixi.toml")
	if _, err := os.Stat(manifest); err != nil {
		return nil, fmt.Errorf(
			"audit coding-agent docs: no pixi manifest at %s: %w", manifest, err)
	}
	if _, err := exec.LookPath("pixi"); err != nil {
		return nil, fmt.Errorf(
			"audit coding-agent docs: pixi is not installed; " +
				"install it from https://pixi.sh and run pixi install")
	}
	pixiArgs := append([]string{"run", "--manifest-path", manifest, "python3"}, args...)
	return exec.Command("pixi", pixiArgs...), nil
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
	fmt.Printf("boot smoke passed for %d coding profiles\n", len(profiles))
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
