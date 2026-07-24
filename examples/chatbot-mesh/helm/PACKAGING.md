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
examples/chatbot-mesh/agents/executor/     -> profiles/agents/executor/       (deployment plane, srd006)
examples/chatbot-mesh/ux/ux.yaml            -> profiles/ux/ux.yaml            (UI descriptor; co-generated key)
examples/chatbot-mesh/ux/app/dist/          -> profiles/ux/app/dist/          (built SPA the chatbot serves at /ui)
```

The ux contributes those two entries, not its whole tree. Every file staged
under `profiles/` becomes a ConfigMap key and a projected mount item in *every*
agent pod, so the staged set is exactly what the chart consumes: `ux.yaml`, and
the bundle the chatbot's `static_assets` binding serves. This document remains
outside that subtree because documentation is not runtime input. The panel
sources, `tsconfig.json`, and the 60 KiB `package-lock.json` are build inputs,
not deployment inputs, and `node_modules` -- present whenever a developer has
run `npm install` -- carries files over helm's 5 MiB per-file limit, which fails
the render outright (GH-702).

The chatbot `rest.yaml`, `ux/ux.yaml`, `request-machine.yaml`, and
`request-fanout.yaml` are co-generated from the `ragUnits` values: the
`profiles-configmap` skips these packaged keys and emits rendered versions in
their place (`templates/_chatbot-rest.tpl`, `_chatbot-ux.tpl`,
`_chatbot-machine.tpl`, `_chatbot-fanout.tpl`), so the deployed topology, the
chatbot client config, and the fan-out breadth (one Retrieving state and one
rag_queryN word per RAG) all share one source of truth. The packaged copies under
the staged subtree remain the local integration source; the render overrides
them in the cluster. The `rag-server` profile is env-parameterized, so the
packaged copy is used verbatim and the chart passes per-pod environment. SPA
assets under `ux/app/dist` (~216 KiB) fit within the 1 MiB ConfigMap limit
alongside the rest of the profile.
