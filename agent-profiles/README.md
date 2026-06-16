# agent-profiles

This repository owns external agent programs and profile assets for
`agent-core`.

Stored here are YAML agent programs, profile-local config, human-facing assets,
demos, and integration fixtures. Runtime code stays elsewhere: Go packages,
builtin tool implementations, the `agent` binary, and release image logic live
in `agent-core`.

## Repository Contract

Profile-owned programs live under `agents/`, grouped by agent family. Runnable
examples belong in `demo/`; integration suites and fixture data belong in
`testdata/integration/`.

Documentation under `docs/` records purpose, structure, indexes, roadmap
entries, and issue format rules. Core-owned runtime assets stay in
`agent-core`.

## Local Usage

Run `agent-core` commands with this checkout as the profile root:

```bash
export AGENT_PROFILES_ROOT="$(pwd)"
agent --profile "$AGENT_PROFILES_ROOT/agents/generator/profile.yaml" --directory /path/to/workspace
```

`agent-core` tests and Mage integration targets use `AGENT_PROFILES_ROOT` to
resolve profiles, demos, and integration fixtures from this repository.

## Container Usage

For containers, callers mount this repository, check it out, or unpack a release
bundle. The image supplies the `agent` binary plus core-owned runtime assets.
Profiles and workspace files come from the caller.

```bash
docker run --rm \
  -v "$PWD:/profiles:ro" \
  -v "$WORKSPACE:/work" \
  agent-core:latest \
  --profile /profiles/agents/generator/profile.yaml \
  --directory /work
```

The mounted `/profiles` path is the container form of `AGENT_PROFILES_ROOT`.

## Validation

This repository is specification and asset focused. Run the documented audit
target when it is available:

```bash
mage audit
```

Until a local Mage target exists, validate YAML syntax for changed docs and
profile assets.
