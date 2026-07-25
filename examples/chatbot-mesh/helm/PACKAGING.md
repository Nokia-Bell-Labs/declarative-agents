# Packaged profiles

The `profiles/` subtree is packaged into the
`<release>-chatbot-mesh-profiles` ConfigMap and projected into each agent at
`/profiles` (nested paths restored from the encoded ConfigMap keys; see
`templates/_helpers.tpl`).

A packaging step copies the agent programs and the ux artifacts the chart
deploys into that directory before `helm package`/`helm install`:

```
examples/chatbot-mesh/agents/chatbot/      -> profiles/agents/chatbot/
examples/chatbot-mesh/agents/rag-server/   -> profiles/agents/rag-server/
examples/chatbot-mesh/agents/coordinator/  -> profiles/agents/coordinator/   (control plane)
examples/chatbot-mesh/agents/creator/      -> profiles/agents/creator/       (control plane)
examples/chatbot-mesh/agents/applier/     -> profiles/agents/applier/       (deployment plane, srd006)
examples/chatbot-mesh/agents/collector/    -> profiles/agents/collector/      (trace ingress, srd007)
examples/chatbot-mesh/agents/corpus-ingest/ -> profiles/agents/corpus-ingest/  (application wrapper + REST values)
agent-profiles/agents/knowledge-manager/corpus-ingest/ -> profiles/agents/knowledge-manager/corpus-ingest/ (canonical program)
examples/chatbot-mesh/ux/ux.yaml            -> profiles/ux/ux.yaml            (UI descriptor; co-generated key)
examples/chatbot-mesh/ux/app/dist/          -> profiles/ux/app/dist/          (built SPA the chatbot serves at /ui)
```

Corpus ingest is the reference-mechanism exception to the otherwise
example-local source list: the mesh owns only a wrapper profile and its
`corpus-rest.yaml` parameterization. Machine, tools, and declarations come from
the canonical `agent-profiles` library directory and are staged at the same
runtime path the wrapper references.

The ux contributes those two entries, not its whole tree. Every file staged
under `profiles/` becomes a ConfigMap key and a projected mount item in *every*
agent pod, so the staged set is exactly what the chart consumes: `ux.yaml`, and
the bundle the chatbot's `static_assets` binding serves. This document remains
outside that subtree because documentation is not runtime input. The panel
sources, `tsconfig.json`, and the 60 KiB `package-lock.json` are build inputs,
not deployment inputs, and `node_modules` -- present whenever a developer has
run `npm install` -- carries files over helm's 5 MiB per-file limit, which fails
the render outright (GH-702).

The chatbot `rest.yaml`, `ux/ux.yaml`, and `request-topology.yaml` are
co-generated from `ragUnits`: the profiles ConfigMap emits rendered versions
through `_chatbot-rest.tpl`, `_chatbot-ux.tpl`, and `_chatbot-topology.tpl`.
The selected-target REST operation, its network allowlist, monitor upstreams,
and ordered runtime topology therefore share one source of truth with the RAG
objects. `request-machine.yaml` and `request-fanout.yaml` are packaged verbatim:
they contain one sequential `for_each`, one `rag_query`, generic partitions, and
`render_each`, so source additions change data but no word or state count. The
`rag-server` profile is env-parameterized, so the
packaged copy is used verbatim and the chart passes per-pod environment. SPA
assets under `ux/app/dist` (~216 KiB) fit within the 1 MiB ConfigMap limit
alongside the rest of the profile.
