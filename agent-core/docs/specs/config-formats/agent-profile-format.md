---
id: cfg-agent-profile-format
title: Agent Profile Format (profile.yaml)
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the YAML format of an agent profile, the file `--profile` names. A
profile bundles everything one agent program needs: its machine, its tool
selection, the tool declarations and REST definitions that supply that
vocabulary, and an optional workspace directory. The same file is the unit a
child agent, a REST `machine_request` binding, staging, and the program
identity refer to.

Until this document the fields were recorded only by the AgentProfile type in
internal/tools/catalog/profile.go and the startup narrative in
cfg-runtime-contract. The profile loader decodes strictly: a keyname outside
the schema below is a load error, so the schema is the whole format.

<!-- field: schema -->
## Schema

<!-- schema_entry: name -->
### name

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

The agent's name. The loader keeps it as a label and never requires it
(application-dsl-spec 1.0 R-PROF-001).

<!-- schema_entry: machine -->
### machine

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Path to the agent's machine file, which may be a machine template
instance (srd054 R1.2). A path to a machine template itself is a load
error (srd054 R1.4).

<!-- schema_entry: tools -->
### tools

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Tool selection files (cfg-tool-selection-format). At least one entry.

<!-- schema_entry: tool_declarations -->
### tool_declarations

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Tool declaration files and units (cfg-tool-declaration-format).

<!-- schema_entry: tool_config_dirs -->
### tool_config_dirs

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Directories whose declaration files are loaded as tool declarations.

<!-- schema_entry: rest_definitions -->
### rest_definitions

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

REST definition files and units (cfg-rest-tool-format).

<!-- schema_entry: rest_config_dirs -->
### rest_config_dirs

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Directories whose files are loaded as REST definitions.

<!-- schema_entry: directory -->
### directory

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Default workspace directory; `--directory` overrides it.

<!-- schema_entry: libraries -->
### libraries

<!-- field: type -->
#### Type

map[string]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Named library roots (srd056-unit-libraries R2). Each name is a lowercase
kebab root name other than `agent-core`; each value is a directory,
resolved against this file's directory. An import under `/opt/<name>/`
anywhere in this profile's closure maps to that directory, and
`--library <name>=<path>` overrides it for one run. Roots are scoped to
this profile's closure; a child-agent profile declares its own.

<!-- field: path_resolution -->
## Path resolution

Relative paths resolve against the directory of the file that holds them.
Paths under the agent-core install prefix (/opt/agent-core) map through the
install root that `--core-root` sets. Paths under `/opt/<name>` map to the directory the profile
declares for that root under `libraries` (srd056 R2.1), and any other
absolute path is used as written.

<!-- field: blueprints -->
## Blueprints

The profile loader implements no blueprints. srd055 describes a blueprint
that declares an agent once with typed parameters, and a profile that
expands it, but no loader for either landed. The strict decoder rejects the
`unit`, `params`, `profile`, and `expand` keynames those documents would
need, so a profile can neither be a blueprint nor expand one
(application-dsl-spec 1.0 R-PROF-002, R-PROF-003). Profiles that share
structure share it through the documents they reference: tool selections and
declarations, and machine files built from stage fragments and machine
templates.

<!-- field: related_documents -->
## Related documents

- [cfg-runtime-contract](runtime-contract.md)
- [cfg-machine-format](machine-format.md)
- [cfg-tool-selection-format](tool-selection-format.md)
- [cfg-tool-declaration-format](tool-declaration-format.md)
- [cfg-rest-tool-format](rest-tool-format.md)
- [srd052-declaration-fragments](../software-requirements/srd052-declaration-fragments.md)
- [srd054-machine-templates](../software-requirements/srd054-machine-templates.md)
- [srd055-agent-blueprints](../software-requirements/srd055-agent-blueprints.md)
- [srd056-unit-libraries](../software-requirements/srd056-unit-libraries.md)

<!-- field: implementation -->
## Implementation

- internal/tools/catalog/profile.go
- internal/load/loader.go
