# Deploying and Running the Chatbot Mesh

The chatbot mesh is a standalone example under
[`examples/chatbot-mesh/`](../examples/chatbot-mesh/README.md). Its deployment
artifacts and runbook live with it, not here. This file is a redirect so older
links resolve.

- Reader's walkthrough of the mesh (both planes, a single turn, reconfiguration):
  [`examples/chatbot-mesh/docs/how-it-works.md`](../examples/chatbot-mesh/docs/how-it-works.md).
- Building and running the example, its audit and `integration:*` targets:
  [`examples/chatbot-mesh/README.md`](../examples/chatbot-mesh/README.md).
- The Helm chart, its values schema, and the values-to-config co-generation:
  [`examples/chatbot-mesh/helm/README.md`](../examples/chatbot-mesh/helm/README.md).

The control-plane deployment API is the executor
([srd006](../examples/chatbot-mesh/docs/specs/software-requirements/srd006-executor.yaml)),
which the creator drives; there is no separate provisioner.
