<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Corpus Audit

We audit this corpus with the specification-corpus audit this repository
ships: the `specification-critic` corpus-audit profile, run by the agent
built from the sibling `agent-core` (GH-190). The profile binds
`docs/corpus-charter.yaml` relative to the directory it audits, so the
charter here decides which checks gate this corpus.

## Where things live

| What | Where |
|---|---|
| Checks that gate this corpus | `docs/corpus-charter.yaml` |
| The agent and the spec tool | built from the sibling `agent-core` into `agent-core/bin` |
| Format requirements this corpus derives from | `derived_from` entries in the SRDs, resolved against this repository |

`mage audit` builds the agent from the sibling `agent-core` with the Go
toolchain — no pin and no download — and resolves `derived_from` entries
against this repository's own tree. A program repository, which has no
sibling, fetches the release its `docs/external-pins.yaml` pins instead:
each tag's `agent` is published for darwin and linux on amd64 and arm64
with `SHA256SUMS`, verified and kept in the user cache.

The spec tool behind `mage fmt`, `mage convert`, and `mage specCheck` is this
repository's own (`magefiles/spec.go`): it lists the tracked documents, reads
initiatives as SRDs, and formats each with the sibling Markdown library,
built beside the agent.

## Running the audit

The entry point is `mage audit`. It runs the repository gate tests
(`go test ./magefiles/...`: gofmt, the 50-line skill limit, the
hierarchy gate, the help pages, and the rest), then agent-core's gates
that hold this application's files to the monorepo's rules (declaration and
Go size, package shape, and the Markdown corpus's canonical form), then runs
the sibling agent over the application root. For that run it writes
`docs/external-checkouts.yaml`, pointing the format source at this
repository, and removes it afterwards. The file is ignored by git; a checkouts file a person
wrote is left as it is and used instead.

The run exits zero and ends in the `Passed` state when no check in the
charter reports an error. A malformed SRD (an acceptance criterion tracing a
requirement item that does not exist, for example) fails at load with exit
2. After the first download the audit is offline: every `derived_from`
entry resolves through local git calls against the cached source.

The parent audit reads structured citations (traces, touchpoints,
`derived_from`). Citations written inside requirement prose, such as
"(srd010 R5)", are not checked by it; keep them resolvable by hand or with a
local check until the parent covers them.

## When the format changes

The format library, the audit, and this corpus are siblings at one tag
(GH-190): a format change and the corpus that satisfies it land in one
change, gated by this same audit. Program repositories move by editing the
pin in their `docs/external-pins.yaml`; `scrum-master doctor` repairs at it.
