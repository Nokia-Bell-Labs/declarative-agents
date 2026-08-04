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

The chatbot `rest.yaml`, `agents/chatbot/ui/ui.yaml`, and `request-topology.yaml` are
co-generated from `ragUnits`: the profiles ConfigMap emits rendered versions
through `_chatbot-rest.tpl`, `_chatbot-ui.tpl`, and `_chatbot-topology.tpl`.
The selected-target REST operation, its network allowlist, monitor upstreams,
and ordered runtime topology therefore share one source of truth with the RAG
objects. `request-machine.yaml` and `request-fanout.yaml` are packaged verbatim:
they contain one sequential `for_each`, one `rag_query`, generic partitions, and
`render_each`, so source additions change data but no word or state count. The
`rag-server` profile is env-parameterized, so the
packaged copy is used verbatim and the chart passes per-pod environment. SPA
assets under `agents/chatbot/ui/app/dist` (~216 KiB) fit within the 1 MiB ConfigMap limit
alongside the rest of the profile.

`mage helm:package` stages only the classified chart source inventory plus the
manifest-derived runtime assets and provenance. Prior `dist/` archives and
generated `profiles/` content are excluded even when packaging is repeated in a
dirty checkout. Before publishing, the target validates the staged closure,
lints and renders the supported values matrix, compares the archive against the
exact staged-file inventory, rejects links and unexpected or missing files,
then lints and renders the `.tgz` independently.
