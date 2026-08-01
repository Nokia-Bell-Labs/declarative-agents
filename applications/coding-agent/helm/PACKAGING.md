# Packaged profiles

The `profiles/` subtree is staged into the chart and projected into each agent at
`/profiles` (nested paths restored from the encoded ConfigMap keys; see
`templates/_helpers.tpl` and `templates/profiles-configmaps.yaml`). It is gitignored,
so every packaging path regenerates and stages it.

`mage helmPrepare` and `mage helm:package` stage three families of profile:

```
serving deployment package (#875)                 -> profiles/{planner,executor,critic}/ + profiles/manifests/
applications/catalog/agents/collector/            -> profiles/collector/agents/collector/
applications/coding-agent/agents/serving/applier/ -> profiles/applier/agents/applier/
```

The serving roles flow through the deterministic `deployment.serving_profiles`
package: `Package` resolves each role's reference closure, partitions it into
ConfigMap-sized shards, and writes a per-role manifest under `profiles/manifests/`
that `templates/profiles-configmaps.yaml` and `_helpers.tpl` read.

The collector and applier are special-cased, mirroring each other: both are staged as
a flat family of files mounted at `/profiles/agents/<name>/`, and neither is a
canonical serving role, so neither enters the serving package. The applier is the
srd006 deployment-plane actuator; its 13 profile files (the lifecycle profile, the
apply and rollout request profiles, the two request machines, the helm and kubectl
exec declarations, and the REST definitions) are self-contained, referencing one
another by bare filename and resolved relative to the mounted profile at runtime.

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

`mage helm:package` stages only the classified chart source inventory plus the three
profile families above. Prior `dist/` archives and generated `profiles/` content are
excluded even when packaging is repeated in a dirty checkout. Before publishing, the
target lints and renders the supported values matrix (every `schema-fixtures/valid-*`
merged over `values.yaml`), compares the archive against the exact staged-file
inventory, rejects links and unexpected or missing files, then lints and renders the
`.tgz` independently.
