---
id: program-specifications
title: Program Specification Index
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

This index covers the system corpus under docs/: the foundation documents, the system SRD, and the releases planned in docs/road-map.md. Each module keeps its own specification corpus and index under its own docs/ folder, which its own audit checks.

<!-- field: roadmap_summary | form: "typed table" -->
## Roadmap summary

| version | name | use_cases_done | use_cases_total | status |
| --- | --- | --- | --- | --- |
| "01.0" | First release | 0 | 0 | specified |

<!-- field: foundation_document_index -->
## Foundation document index

| id | path |
| --- | --- |
| [program-vision](VISION.md) | docs/VISION.md |
| [program-architecture](ARCHITECTURE.md) | docs/ARCHITECTURE.md |
| [design](constitutions/design.md) | docs/constitutions/design.md |
| [program-roadmap](road-map.md) | docs/road-map.md |

<!-- field: srd_index -->
## SRD index

| id | path | title | release |
| --- | --- | --- | --- |
| [srd001-system](specs/software-requirements/srd001-system.md) | docs/specs/software-requirements/srd001-system.md | System: declarative-agents | 01.0 |

<!-- field: config_format_index -->
## Config format index

<!-- field: semantic_model_index -->
## Semantic model index

<!-- field: use_case_index -->
## Use case index

<!-- field: test_suite_index -->
## Test suite index

<!-- field: coverage_gaps -->
## Coverage gaps

| id | gap |
| --- | --- |
| GAP1 | The system corpus has no use cases or test suites; its requirements are proven by each module's own gates, which srd001's acceptance criteria name. |
