# rag-server (relocated)

The RAG server agent is no longer maintained in the agent-profiles catalog. The
mesh was extracted to a standalone example, and the RAG server program now lives,
as the single canonical copy, at:

    examples/chatbot-mesh/agents/rag-server/

The catalog copy here was a near-duplicate that had diverged from the example, so
it was removed to end the duplication (GH-511). The example's integrations and
Helm chart consume the canonical copy.

The agent-profiles rel09 mesh specifications are the historical record of the
mesh's development here before extraction; see examples/chatbot-mesh/docs for the
canonical specifications.
