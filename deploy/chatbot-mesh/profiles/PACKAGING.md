# Packaged profiles

This subtree is packaged into the `<release>-chatbot-mesh-profiles` ConfigMap and
projected into each agent at `/profiles` (nested paths restored from the encoded
ConfigMap keys, see `templates/_helpers.tpl`).

A packaging step copies the agent programs the chart deploys into this directory
before `helm package`/`helm install`:

```
agent-profiles/agents/chatbot/            -> profiles/agents/chatbot/
agent-profiles/agents/chroma/rag-server/  -> profiles/agents/chroma/rag-server/
```

GH-314 co-generates the chatbot `rest.yaml` RAG client entries from the `ragUnits`
values into this subtree, so the deployed topology and the chatbot client config
share one source of truth. Large SPA assets under `agents/chatbot/ui/app/dist`
exceed the 1 MiB ConfigMap limit and are handled by a separate asset volume in a
follow-on; the chart topology (this issue, GH-313) renders and lints without them.
