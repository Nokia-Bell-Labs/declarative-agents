# Catalog tool blocks

`applications/catalog` is this repository's catalog of reusable declarative
blocks. It is not a universal tool registry. Tool declarations are first-class
catalog assets when they configure reusable repository-specific behavior, but
generic implementations and core vocabulary remain owned by `agent-core`.

## Taxonomy

The repository uses five distinct artifact classes:

1. **Core implementations** live under `agent-core/internal/tools/`. Go
   commands, builders, adapters, and factories execute behavior. They do not
   move into this catalog when a catalog profile selects them.
2. **Core shared declarations** live under `agent-core/tools/` and are installed
   at `/opt/agent-core/tools`. They define stable runtime vocabulary shared
   across unrelated profiles, such as filesystem, lifecycle, LLM, and exec
   words.
3. **Catalog tool blocks** are reusable repository-specific `ToolDef`
   declarations owned by a catalog family under `agents/<family>/`. Examples
   include `agents/critic/builtin.yaml`, `agents/monitor/declarations.yaml`, and
   `agents/executor/llm/default.yaml`. A block defines vocabulary, boundary
   configuration, signals, side effects, errors, and undo behavior; it does not
   own the Go implementation selected by `init`.
4. **Tool selection lists** are `tools.yaml` files. They contain names chosen
   for one machine or profile. A selection grants no new behavior and is not a
   declaration block.
5. **Profile-local overrides** are declarations loaded by a profile to narrow
   or configure selected vocabulary for that profile or variant. Model
   declarations under `agents/executor/llm/` are representative. An override is
   part of its owning profile closure; it does not replace the canonical core
   declaration for other consumers.

`MachineSpec` remains the owner of sequencing. One `ToolDef` remains one
vocabulary word. Composition belongs in machines and profiles, not inside a
multi-step tool declaration.

## Catalog membership

A catalog tool block must:

- be independently reusable in this repository or serve more than one real
  consumer;
- have one canonical home under its owning `agents/<family>/` closure;
- declare its inputs, outputs, emitted signals, side effects, errors, and undo
  or compensation behavior;
- use portable profile-relative references or documented core mount paths;
- have deterministic catalog conformance and an SRD when it exposes public
  behavior; and
- be versioned with the containing catalog member.

A reusable agent block is the closed composition of its profile, machines, tool
selection lists, catalog declarations, references to core declarations, and
boundary configuration. Applications reference or package that closure; they
do not copy its reusable machines or declarations.

## Application-local exclusions

The following stay under `applications/<application>/`:

- topology, endpoint choices, role wiring, and deployment policy;
- application wrappers and composition manifests;
- credentials, environment-specific values, and operator UX;
- packaging, charts, images, and end-to-end fixtures; and
- declarations or overrides useful only to that application.

Being YAML does not make an artifact a catalog member. Promote application-local
behavior only after it satisfies the reuse and evidence obligations above.
Core-only integration fixtures under `agent-core/testdata/integration/` and
catalog conformance scaffolding under `testdata/conformance/` are also not
catalog members.

## Dependency direction

The dependency direction is:

```text
applications/<name>
  -> applications/catalog agent and tool blocks
  -> agent-core shared declarations and runtime contracts
  -> agent-core Go implementations
```

`agent-core` never imports catalog or application products. The catalog may
select core vocabulary but does not own core implementations. Applications may
configure catalog blocks but must not fork their reusable behavior.

## Compatibility and migration

| Surface | Historical value | Current value |
|---|---|---|
| Source root | `agent-profiles/` | `applications/catalog/` |
| Runnable products | `examples/<name>/` | `applications/<name>/` |
| Catalog environment | `AGENT_PROFILES_ROOT` | `AGENT_CATALOG_ROOT` |
| Catalog module tag | `agent-profiles/v0.*` | `applications/catalog/v0.*` |
| Packaged profile mount | `/profiles` | `/profiles` (unchanged) |
| Core declaration mount | `/opt/agent-core/tools` | `/opt/agent-core/tools` (unchanged) |

During Release 99.0, `AGENT_PROFILES_ROOT` is a deprecated alias used only when
`AGENT_CATALOG_ROOT` is unset. If both are set, they must resolve to the same
absolute path. Existing `agent-profiles/v0.*` tags remain immutable, and
coordinated v0 releases publish matching canonical and compatibility tags.
New documentation and manifests use `applications/catalog/v0.*`.

Run commands from the owning module root:

```bash
# repository root
mage audit

# applications/catalog
AGENT_CORE_ROOT="$(git rev-parse --show-toplevel)/agent-core" mage validate
mage conformance
mage integration:all

# applications/coding-agent or applications/chatbot-mesh
AGENT_CATALOG_ROOT="$(git rev-parse --show-toplevel)/applications/catalog" mage audit
```

Source roots, Go module paths, release tags, packaged closure destinations, and
runtime mounts are separate contracts. A source-path migration must not rewrite
`/profiles` or `/opt/agent-core/tools`.
