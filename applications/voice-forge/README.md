# Voice Forge

Voice Forge is a planned, documentation-first declarative-agent application for
editing a source document without losing its meaning or provenance. Its release
00.0 scenario captures an original from GitHub, improves its structure, matches
a target voice, tightens its style, obtains an independent critique, and
publishes the accepted result through one commit, push, and pull request.

The planned workflow is:

1. A human supplies the source repository, path, revision, target voice, and
   external-upload consent.
2. The workflow orchestrator captures an immutable original and starts a local
   saga under `workproducts/<saga-id>/`.
3. The structure editor returns a structure candidate that the orchestrator
   stores as an immutable attempt.
4. The voice editor uses read-only retrieval and returns a voice candidate.
5. The style editor uses retrieved tightening examples and returns a style
   candidate.
6. The voice critic independently compares the original and candidate. It may
   call Pangram only when the human has consented to both uploads.
7. On acceptance, the orchestrator materializes `10-structure.md`,
   `20-voice.md`, `30-style.md`, `40-critique.yaml`, and `final.md`, commits the
   complete workproduct set once, pushes, and opens or reuses a pull request.
   Before acceptance, every workproduct remains local and uncommitted.

The application plans six agent services: `workflow-orchestrator`,
`structure-editor`, `voice-editor`, `style-editor`, `voice-critic`, and a
wrapper around the canonical `corpus-ingest` program. Structure, voice, and
style RAG server instances provide read-only retrieval over separate corpora.
Chroma is a rebuildable index, Ollama is the model boundary, GitHub is the
source and publication boundary, and Pangram is a consent-gated advisory
service available only to the critic.

This directory currently contains documentation only. No profile, machine,
tool, service, test, package, registry entry, or runnable composition is
implemented or claimed. Release `00.0`, SRDs `srd001` through `srd008`, use case
`rel00.0-uc001-edit-document`, and test suite
`test-rel00.0-voice-forge` are planned.

The design extends the shared application contract in `applications/docs/` and
the runtime contracts in `agent-core/docs/specs/` by reference. It does not
copy or replace those requirements. See `docs/VISION.yaml`,
`docs/ARCHITECTURE.yaml`, `docs/SPECIFICATIONS.yaml`, and
`docs/road-map.yaml`.
