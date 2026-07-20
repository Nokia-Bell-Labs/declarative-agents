# Deploying and Running the Chatbot Mesh

This directory holds the deployment artifacts for the chatbot mesh (agent-core
srd016 trace propagation, agent-profiles srd013 RAG server, srd014 chatbot,
srd015 deployment). It is also the runbook for standing up the mesh, or any
single agent, on a fresh machine for development and verification.

The mesh is a set of container agents: a browser-facing chatbot agent that fans
one query embedding out to N RAG server agents (Chroma-backed) and routes to a
chat LLM. The Helm chart now lives with the standalone example at
`examples/chatbot-mesh/helm/`, and the provisioner (control-plane deployment API)
with it at `examples/chatbot-mesh/provisioner/`. This README covers both the local
dev loop and the Kubernetes path.

## Prerequisites

The versions below are the ones this environment was verified against. Newer
patch releases are expected to work.

| Tool | Verified | Used for |
|---|---|---|
| Go | 1.26 | Building the agent binary and running unit/integration tests |
| mage | 1.17 | `validate`, `audit`, `test`, `dolt:*` targets |
| Docker | 29 | Persistent Dolt, Chroma, and (in cluster) every agent |
| Ollama | recent | Local embedding and chat models |
| kind | 0.32 | Local Kubernetes cluster for the Helm smoke test |
| helm | 4.2 | Rendering and installing the mesh chart |
| kubectl | recent | Inspecting the cluster |
| node / npm | 26 / 11 | Building the chatbot SPA panels |

Install on macOS with Homebrew: `brew install go mage kind helm kubectl node`,
plus Docker Desktop and the Ollama app.

## Repository Layout

Check out `agent-core` and `agent-profiles` as siblings under one parent. The
profile validation and integration targets resolve agent-core as `../agent-core`
(or through `AGENT_CORE_ROOT`), and profiles reference installed-core assets
under `/opt/agent-core`, which the agent maps to a development checkout through
the `--core-root` flag.

```
GITHUB/
  agent-core/       # runtime, cmd/agent, mage targets
  agent-profiles/   # agent programs (chroma, chatbot, ...), mage validate/audit
```

## Backing Services

The agents reach three local services over loopback. Start them before running
an agent or the integration tests.

### Persistent Dolt (checkpoints)

Dolt stores lifecycle checkpoints so a run can suspend, resume, and roll back.
Storage persists across container removal in a named volume (see
`agent-core/docker-compose.dolt.yml`).

```bash
cd agent-core
mage dolt:up        # start on 127.0.0.1:3306, root reachable over TCP
mage dolt:status    # service + volume
mage dolt:down      # stop, keep the data
mage dolt:reset     # stop and discard the data
```

Point an agent at it with `--dolt-dsn "root@tcp(127.0.0.1:3306)/<database>"`.
Without `--dolt-dsn` the agent uses an in-memory NoopCheckpoint and keeps no
durable history.

### Chroma (RAG vector store)

Each RAG server serves one Chroma collection.

```bash
docker run -d --name chroma -p 8000:8000 chromadb/chroma:latest
curl -s http://localhost:8000/api/v2/heartbeat   # readiness
```

The corpus is loaded out of band by the `agents/knowledge-manager/corpus-ingest` profile; the RAG
server and reader only serve an already-ingested collection named `corpus` under
`default_tenant/default_database`.

### Ollama (embeddings and chat)

The ingest, reader, and chatbot agents embed at Ollama and answer with a local
chat model. The RAG server's vector-in query path does not need Ollama (the
caller supplies the query vector).

```bash
ollama serve                       # or the Ollama app; listens on 11434
ollama pull qwen3-embedding:8b     # embedding model used by ingest/reader
ollama pull ornith:9b              # answer model used by the reader
```

## Build and Run an Agent

Build the binary from agent-core, then run a profile. `--core-root` maps the
profile's `/opt/agent-core` references to the development checkout; `--directory`
is the workspace the tools operate on.

```bash
cd agent-core
go build -o bin/agent ./cmd/agent

# Example: the documentation-curator server profile
AGENT_PROFILES_ROOT=../agent-profiles \
bin/agent \
  --profile ../agent-profiles/agents/knowledge-manager/documentation-curator/profile.yaml \
  --core-root "$PWD" \
  --directory /tmp/workspace
```

Server profiles bind loopback ports declared in their `rest.yaml`. For the RAG
server (`agents/knowledge-manager/rag-server`) these are the query endpoint on 18085, the
control server (health + lifecycle exit) on 18086, and the monitor server (state
views + SSE) on 18087. Check health and request a graceful exit:

```bash
curl -s http://localhost:18086/api/lifecycle/health
curl -s -X POST http://localhost:18086/api/lifecycle/exit -d '{"reason":"done"}'
```

## Validate and Test

```bash
# agent-profiles: profile wiring and declaration checks
cd agent-profiles && mage validate && mage audit

# agent-core: spec traceability audit and unit tests
cd ../agent-core && mage audit && go test ./...

# Gated Dolt integration tests run when mage dolt:up is up, skip otherwise
go test ./cmd/agent -run 'TestDoltCheckpoint|TestDoltCommandState'
```

## Kubernetes Deployment

The data-plane mesh chart lives with the standalone example at
`examples/chatbot-mesh/helm/`. It deploys the chatbot, N values-driven RAG
(agent + Chroma) pairs, the chat LLM, the Dolt backend, and an OTel collector
over binary-only images with profiles mounted from ConfigMaps, and co-generates
the chatbot config from the RAG list. The smoke path is a local kind cluster:

```bash
kind create cluster --name chatbot-mesh
helm install chatbot-mesh examples/chatbot-mesh/helm \
  -f examples/chatbot-mesh/helm/ci/kind-values.yaml
kubectl get pods
kind delete cluster --name chatbot-mesh
```

See `examples/chatbot-mesh/helm/README.md` for the chart, its values schema, and
the values-to-config co-generation. The provisioner (control-plane deployment
API) lives at `examples/chatbot-mesh/provisioner/`.

## Current Status

This section records what a maintainer can rely on today versus what is still in
progress, so a fresh checkout is not misread as fully working.

| Component | Status |
|---|---|
| Persistent Dolt (`mage dolt:*`) | Verified end to end |
| Chroma container + vector query | Verified (direct API and seeded collection) |
| Agent build + server lifecycle | Verified (health, monitor, control all serve) |
| Spec docs (rel08.0, rel09.0) | Merged; audits clean |
| RAG server request path | Blocked — see below |
| Chatbot agent, routing, ux SPA | Implemented in `examples/chatbot-mesh/` |
| Helm chart + kind smoke test | Implemented in `examples/chatbot-mesh/helm/` |
| Ollama-gated integration tracers | Require the models above; not run here |

Known blocker (RAG server query path): a `machine_request` seed is a structured
JSON object carrying `method` and `path`, and every REST-client word rejects any
runtime input containing those transport-authority field names. A REST-client
word therefore cannot be the first word of a machine_request-seeded request
machine. The RAG server profile validates and its lifecycle serves, but its query
chain needs an agent-core enabling change before it answers. This is tracked
against the RAG server implementation issue.
