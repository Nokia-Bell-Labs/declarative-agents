# Packaged profiles

The `profiles/` subtree is staged into the chart and projected into each agent at
`/profiles` (nested paths restored from the encoded ConfigMap keys; see
`templates/_helpers.tpl` and `templates/profiles-configmaps.yaml`). It is gitignored,
so every packaging path regenerates and stages it.

`mage helmPrepare` and `mage helm:package` stage the manifest-derived deployment
closures plus the catalog collector:

```
agents/application.yaml deployment entries        -> profiles/{planner,executor,critic,applier}/ + profiles/manifests/
applications/catalog/agents/collector/            -> profiles/collector/agents/collector/
```

`Package` validates the shared Release 14 manifest, resolves each deployment
entry's reference closure, partitions it into ConfigMap-sized shards, and writes
a per-entry manifest under `profiles/manifests/`. The serving templates consume
the planner, executor, and critic manifests; the privileged applier template
consumes the applier shard.

Only the collector remains separately staged because it is catalog-owned
observability support rather than an application deployment entry. The local
applier is rooted at `agents/applier/profile.yaml`, packaged at
`/profiles/applications/coding-agent/applier/profile.yaml`, and retains its
self-contained relative request-machine references.

## Exec placeholder rewrite

The applier's `exec-declarations.yaml` carries static placeholder coordinates -- the
release name `coding-agent`, the namespace `default`, and the
`coding-agent-{planner,executor,critic}` Deployments -- kept as valid YAML so
agent-core validates the profile. `templates/applier.yaml` rewrites them at render
time to the installed `$.Release.Name`, `$.Release.Namespace`, and
`<fullname>-{planner,executor,critic}` Deployments, so an applier installed under any
release name targets its own release. The deployment tokens are rewritten before the
bare release tokens because the deployment names contain the release-chart name.

## Package target

`mage helm:package` stages only the classified chart source inventory, generated
manifest deployment shards, and collector profile. Prior `dist/` archives and generated `profiles/` content are
excluded even when packaging is repeated in a dirty checkout. Before publishing, the
target lints and renders the supported values matrix (every `schema-fixtures/valid-*`
merged over `values.yaml`), compares the archive against the exact staged-file
inventory, rejects links and unexpected or missing files, then lints and renders the
`.tgz` independently.
