# Packaged profiles

This subtree is packaged into the `<release>-chatbot-mesh-profiles` ConfigMap and
projected into each agent at `/profiles` (nested paths restored from the encoded
ConfigMap keys, see `templates/_helpers.tpl`).

A packaging step copies the agent programs and the ux the chart deploys into this
directory before `helm package`/`helm install`:

```
examples/chatbot-mesh/agents/chatbot/      -> profiles/agents/chatbot/
examples/chatbot-mesh/agents/rag-server/   -> profiles/agents/rag-server/
examples/chatbot-mesh/agents/coordinator/  -> profiles/agents/coordinator/   (control plane)
examples/chatbot-mesh/agents/creator/      -> profiles/agents/creator/       (control plane)
examples/chatbot-mesh/agents/executor/     -> profiles/agents/executor/       (deployment plane, srd006)
examples/chatbot-mesh/ux/                   -> profiles/ux/
```

The chatbot `rest.yaml`, `ux/ux.yaml`, `request-machine.yaml`, and
`request-fanout.yaml` are co-generated from the `ragUnits` values: the
`profiles-configmap` skips these packaged keys and emits rendered versions in
their place (`templates/_chatbot-rest.tpl`, `_chatbot-ux.tpl`,
`_chatbot-machine.tpl`, `_chatbot-fanout.tpl`), so the deployed topology, the
chatbot client config, and the fan-out breadth (one Retrieving state and one
rag_queryN word per RAG) all share one source of truth. The packaged copies under
this subtree remain the local integration source; the render overrides them in the
cluster. The `rag-server` profile is env-parameterized, so the packaged copy is
used verbatim and the chart passes per-pod environment. SPA assets under `ux/app/dist`
(~216 KiB) fit within the 1 MiB ConfigMap limit alongside the rest of the profile.
