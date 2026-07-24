//go:build mage

// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Audit validates the coding application's documentation scaffold. Integration
// evidence is added with the coding-loop implementation.
func Audit() error {
	const validator = `
import pathlib
import sys
import yaml

root = pathlib.Path(sys.argv[1])
docs = root / "docs"
paths = sorted(docs.rglob("*.yaml"))
loaded = {path: yaml.safe_load(path.read_text()) for path in paths}

required = {
    docs / "VISION.yaml": {
        "id", "title", "executive_summary", "problem", "what_this_does",
        "why_we_build_this", "success_criteria", "not",
    },
    docs / "ARCHITECTURE.yaml": {
        "id", "title", "overview", "interfaces", "components",
        "design_decisions", "technology_choices", "project_structure",
        "implementation_status", "related_documents",
    },
    docs / "road-map.yaml": {"id", "title", "releases"},
    docs / "SPECIFICATIONS.yaml": {
        "id", "title", "overview", "roadmap_summary",
        "foundation_document_index", "srd_index", "config_format_index",
        "semantic_model_index", "use_case_index", "test_suite_index",
        "coverage_gaps",
    },
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
cases = suite.get("test_cases", [])
if [case.get("stage") for case in cases] != ["A", "B", "C"]:
    errors.append("test suite must define ordered stages A, B, and C")
for case in cases:
    if not case.get("inputs") or not case.get("expected_outputs"):
        errors.append(f"{case.get('name', 'unnamed case')} lacks inputs or expected_outputs")

if errors:
    raise SystemExit("\n".join(errors))
print(f"audit: parsed and validated {len(paths)} coding-agent YAML documents")
`

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd := exec.Command("python3", "-c", validator, root)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("audit coding-agent docs: %w", err)
	}
	return nil
}
