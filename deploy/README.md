# Deploying and Running the Chatbot Mesh

The chatbot mesh is a standalone example under
[`applications/chatbot-mesh/`](../applications/chatbot-mesh/README.md). Its deployment
artifacts and runbook live with it, not here. This file is a redirect so older
links resolve.

- Reader's walkthrough of the mesh (both planes, a single turn, reconfiguration):
  [`applications/chatbot-mesh/docs/how-it-works.md`](../applications/chatbot-mesh/docs/how-it-works.md).
- Building and running the example, its audit and `integration:*` targets:
  [`applications/chatbot-mesh/README.md`](../applications/chatbot-mesh/README.md).
- The Helm chart, its values schema, and the values-to-config co-generation:
  [`applications/chatbot-mesh/helm/README.md`](../applications/chatbot-mesh/helm/README.md).
- The browser prerequisite for driving any UI headless (we use `puppeteer-core`,
  so the host supplies Chrome and names it in `PUPPETEER_EXECUTABLE_PATH` or
  `CHROME_BIN`):
  [`agent-core/README.md`](../agent-core/README.md#browser-end-to-end-tests).

The control-plane deployment API is the applier
([srd006](../applications/chatbot-mesh/docs/specs/software-requirements/srd006-applier.yaml)),
which the creator drives; there is no separate provisioner.
