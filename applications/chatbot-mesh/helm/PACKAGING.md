# Packaged profiles

The `profiles/` subtree supplies the shared
`<release>-chatbot-mesh-profiles` ConfigMap projected into each agent at
`/profiles` (nested paths restored from the encoded ConfigMap keys; see
`templates/_helpers.tpl`). The observer UI remains at its established packaged
path below `profiles/`, but the chart places that bundle in an observer-only
ConfigMap and mounts it back at the same runtime path.

A packaging step loads `agents/application.yaml`, resolves its deterministic
transitive closure with the shared `appmanifest` package, and copies only the
inventory files into the chart before `helm package`/`helm install`. Local and
catalog profile roots, deployment entry profiles, runtime paths, and the
chatbot, observer, and collector UI assets therefore have one composition
authority. There is no Helm-owned agent list and no whole-actor copy followed by
fixture or UI-development pruning.

Files with `agents/...` runtime paths are staged below `profiles/` for the
manifest's `/profiles` mount. The collector UI declares the external
`collector-ui/ui/dist` runtime destination used by its dedicated ConfigMap.
`provenance/application-closure.yaml` records the composition-manifest checksum,
the application and catalog checkout revisions and dirty states when available,
sorted direct-root compatibility provenance, every source/runtime/package
mapping, and each content checksum. Package and archive validation require every
recorded file.

Corpus ingest is a manifest-declared catalog consumer: the mesh owns only a
wrapper profile and its `corpus-rest.yaml` parameterization. Machine, tools, and
declarations come from the canonical `applications/catalog` directory and are
staged at the same runtime path the wrapper references. The collector profile
and trace UI are catalog-owned roots as well. The `demo.yaml` catalog_root
selects that checkout. In-tree builds discover the repository's
`applications/catalog` directory. The checkout is required to
build and test the closure, while the resulting chart archive contains all
resolved files and needs no profile checkout at runtime.

`Chart.yaml` records
`declarative-agents.nokia.com/catalog-compatible-release:
v0.20260804.0`. The composition manifest uses the same current root
`v0.20260804.0` compatibility identifier for each catalog-owned root. The value
is a compatibility pin, not a claim that an arbitrary source checkout is the
immutable release. The exact root catalog tag is published from `main` after
merge; packaging on this branch stages the reviewed checkout and does not create release tags.

The chatbot and observer UIs contribute their built runtime entries, not their
whole package trees. The chatbot bundle remains in the shared profiles
ConfigMap. The larger observer React bundle is excluded from that shared object
and mounted only into the observer from `<release>-chatbot-mesh-observer-ui`;
its archive and runtime location remain `profiles/agents/observer/ui/dist`.
This document remains outside the runtime subtree because documentation is not
runtime input. Panel sources, `tsconfig.json`, package lockfiles, and
`node_modules` are build inputs rather than deployment inputs.

The chatbot `rest.yaml`, `agents/chatbot/ui/ui.yaml`, and
`request-topology-declarations.yaml` are co-generated from `ragUnits`: the
profiles ConfigMap emits rendered versions
through `_chatbot-rest.tpl`, `_chatbot-ui.tpl`, and `_chatbot-topology.tpl`.
The selected-target REST operation, its network allowlist, monitor upstreams,
and ordered runtime topology therefore share one source of truth with the RAG
objects. `request-machine.yaml` and `request-fanout-declarations.yaml` are
packaged verbatim: they contain one sequential `for_each`, one `rag_query`, generic partitions, and
`render_each`, so source additions change data but no word or state count. The
`rag-server` profile is env-parameterized, so the
packaged copy is used verbatim and the chart passes per-pod environment. SPA
assets under `agents/chatbot/ui/app/dist` (~216 KiB) fit within the 1 MiB ConfigMap limit
alongside the rest of the profile.

## Live release assets and budget

Helm stores a release by JSON-encoding it, gzip-compressing it, and base64
encoding the result into `Secret.data.release`; Kubernetes validates that inner
encoded value against its 1 MiB Secret limit. The chart files and rendered
manifests therefore both charge immutable files that a template copies into a
ConfigMap.

Live integration releases may keep immutable archives outside release storage.
The shared externalization path currently moves the collector and observer UIs
for the applier live tier; later tiers can use the same release-parameterized
helpers without changing the canonical package. Applier live keeps three
archives outside its release:

- the packaged chart in `<release>-applier-chart`, referenced only by
  `applier.chartArchiveConfigMap` (never supplied with `--set-file`);
- the collector UI in the checksum-addressed ConfigMap named by
  `collector.uiArchiveConfigMap`;
- the observer UI in the checksum-addressed ConfigMap named by
  `observer.uiArchiveConfigMap`.

For the selected release name, live staging verifies every UI file against
`provenance/application-closure.yaml`, creates deterministic `assets.tgz`
archives, removes only those verified UI files from the staged chart, and
pre-creates release-prefixed, checksum-addressed ConfigMaps with `kubectl
create`. Each workload receives the archive through an explicit volume, verifies
`uiArchiveChecksum`, and unpacks it at its explicitly configured serving root
(`/collector-ui` or `/observer-ui`); the observer keeps this writable staging
mount beside its read-only `/profiles` mount so it cannot mask the profile
itself. Pod annotations bind rollouts to the archive checksum. An
archive-internal checksum manifest verifies every extracted file against the
manifest-derived package inventory before the workload starts.

The checked-in defaults remain empty, so the ordinary packaged chart stays
self-contained and renders its UI ConfigMaps in-release. Deploy tooling that
sets an external archive name must also set its 64-character SHA-256 checksum
and pre-create a ConfigMap with an `assets.tgz` key.

Before any ConfigMap or Helm release is created, each adopting live path passes
its release name and exact install value arguments to the shared projection. The
projection renders those inputs and measures chart files, values, and manifests
using Helm's gzip/base64 storage behavior. It adds a 64 KiB projection allowance
and requires the result to remain at or below 896 KiB, reserving at least 128 KiB
below Kubernetes' 1 MiB limit. Post-install assertions apply the same budget to
the selected release's actual Helm Secrets and reject any stored external
archive. The package test records both the release-resident baseline and
externalized measurements and fails deterministically on budget regression.

`mage helm:package` stages only the classified chart source inventory plus the
manifest-derived runtime assets and provenance. Prior `dist/` archives and
generated `profiles/` content are excluded even when packaging is repeated in a
dirty checkout. Before publishing, the target validates the staged closure,
lints and renders the supported values matrix, compares the archive against the
exact staged-file inventory, rejects links and unexpected or missing files,
then lints and renders the `.tgz` independently.
