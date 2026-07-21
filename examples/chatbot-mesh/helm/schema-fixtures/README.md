# values.schema.json fixtures

These values files exercise `helm/values.schema.json`, the schema that encodes the
deployment-values constraints the executor agent (srd006) relies on `helm` to
enforce — the declarative deployment API (srd006), replacing the standard-library provisioner
`MeshView.Validate`. A `helm upgrade --dry-run` against this schema is the
executor's validate step (rel06.0-uc001 F2).

Each file is a partial override merged over `values.yaml`. `valid-*` must lint
clean; `bad-*` must be rejected by the schema. Run:

```
helm lint helm -f helm/schema-fixtures/<file>.yaml
```

| Fixture | Rule exercised (MeshView.Validate parity) |
|---|---|
| valid-add-rag.yaml | a well-formed add-a-RAG patch lints clean |
| dup-rag-name.yaml | ragUnit names must be unique (chart-render fail guard, GH-465) |
| bad-rag-name.yaml | rag name must match `^[a-z]([-a-z0-9]*[a-z0-9])?$` |
| bad-nresults.yaml | executor.params.nResults must be >= 1 |
| incluster-missing-chat.yaml | the in-cluster tier requires >= 1 chat model |
| external-missing-url.yaml | disabling ollama requires a non-empty llm.externalURL |
