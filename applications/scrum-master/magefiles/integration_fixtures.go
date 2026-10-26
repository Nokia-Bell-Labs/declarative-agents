// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// seedSystem commits the release 01.0 fixtures to the rig's system
// repository: a road-map release naming objective items, the agreed
// objective document whose R1 is delivered by po-1, and one subsystem SRD
// per Product Owner repository. It tags the seed branch and adds the
// release milestone to the rig group.
func (r *rigEnv) seedSystem(system, po1, po2 string) error {
	doc, err := os.ReadFile(filepath.Join(r.root, "testdata/objectives/valid/obj001-reliable-agents.md"))
	if err != nil {
		return err
	}
	objective := strings.ReplaceAll(string(doc), "https://gitlab.example.com/program/delivery/fleet", r.webBase+"/"+po1)
	roadMap := `---
id: rig-roadmap
title: Regression rig road map
---

<!-- release: ` + rigRelease + ` -->
## ` + rigRelease + `

<!-- field: deliverables -->
### Deliverables

- obj001-reliable-agents#R1.1
- obj001-reliable-agents#R1.2
- obj001-reliable-agents#R2.1

<!-- field: release -->
### Release

<!-- field: publication | form: "tree" -->
#### Publication

- tag: ` + rigTag + `
- milestone: ` + rigMilestone + `
`
	files := map[string]string{
		"docs/road-map.md": roadMap,
		"docs/specs/objectives/obj001-reliable-agents.md": objective,
		"docs/specs/software-requirements/srd010-po-1.md": subsystemSRD("srd010-po-1", "po-1", r.webBase+"/"+po1),
		"docs/specs/software-requirements/srd011-po-2.md": subsystemSRD("srd011-po-2", "po-2", r.webBase+"/"+po2),
	}
	if err := r.seed(system, files); err != nil {
		return err
	}
	if err := r.tag(system, rigTag); err != nil {
		return err
	}
	return r.milestone(rigMilestone, rigDue)
}

// subsystemSRD is a subsystem SRD in the corpus's Markdown format.
func subsystemSRD(id, component, repository string) string {
	return `---
id: ` + id + `
title: "Subsystem: ` + component + `"
---

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, ` + component + `

<!-- field: problem -->
## Problem

The rig's ` + component + ` subsystem.

<!-- field: goals -->
## Goals

- G1: The subsystem receives its objectives.

<!-- req: R1 -->
## R1 - Receive objectives

<!-- req: R1.1 -->
### R1.1

Objective issues reach this repository.

<!-- field: non_goals -->
## Non-goals

- Anything else.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

An objective issue arrives.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| ` + component + ` | ` + repository + ` | / |
`
}

func writeFileTo(path, content string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func writeJSONTo(path string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	writeFileTo(path, string(b))
}

func ptr[T any](v T) *T { return &v }
