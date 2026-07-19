# Packaged profiles

This subtree is packaged into the `<release>-chatbot-mesh-profiles` ConfigMap and
projected into each agent at `/profiles` (nested paths restored from the encoded
ConfigMap keys, see `templates/_helpers.tpl`).

A packaging step copies the agent programs the chart deploys into this directory
before `helm package`/`helm install`:

```
agent-profiles/agents/chatbot/            -> profiles/agents/chatbot/
agent-profiles/agents/knowledge-manager/rag-server/  -> profiles/agents/knowledge-manager/rag-server/
```

The chatbot `rest.yaml`, `ui/ux.yaml`, `request-machine.yaml`, and
`request-fanout.yaml` are co-generated from the `ragUnits` values (GH-314,
GH-372): the `profiles-configmap` skips these packaged keys and emits rendered
versions in their place (`templates/_chatbot-rest.tpl`, `_chatbot-ux.tpl`,
`_chatbot-machine.tpl`, `_chatbot-fanout.tpl`), so the deployed topology, the
chatbot client config, and the fan-out breadth (one Retrieving state and one
rag_queryN word per RAG) all share one source of truth. The packaged copies under this subtree remain the local
integration source; the render overrides them in the cluster. The `rag-server`
profile is env-parameterized (GH-369), so the packaged copy is used verbatim and
the chart passes per-pod environment. SPA assets under `agents/chatbot/ui/app/dist`
(~216 KiB) fit within the 1 MiB ConfigMap limit alongside the rest of the profile.
