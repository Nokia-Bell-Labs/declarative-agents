---
id: catalog
title: Catalog Constitution
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

We bind the shared catalog (applications/catalog) to the specification (application-dsl-spec/): what a published block guarantees to the modules that reference it. Each obligation cites a statement identifier; this constitution introduces no grammar of its own.

<!-- field: obligations -->
## Obligations

<!-- field: publication -->
### Publication

- Every published agent-profile satisfies the agent-profile grammar (R-PROF-001), and every published machine-profile satisfies the machine-profile grammar (R-MACH-001), so a canonical reference never imports a non-conforming document.
- A stage-fragment or machine-template the catalog ships composes under the expansion statements (R-MODEL-001, R-MODEL-002, R-MODEL-003), a published template instance carries only its header (R-MACH-002), and the catalog's conformance suite exercises the expansions its blocks declare.

<!-- field: compatibility -->
### Compatibility

- A root that references the catalog does so through the canonical ownership and compatible_release keynames of the application-profile grammar (R-APP-001); the catalog keeps a published block loadable by every release its consumers pin as compatible, and a breaking change to a published block lands as a new block or a migration, never as an in-place mutation.

<!-- field: references -->
## References

- application-dsl-spec/1.0/language.yaml
- application-dsl-spec/1.0/06-mechanized-checking.md
