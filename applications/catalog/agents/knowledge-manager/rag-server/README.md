<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# rag-server (relocated)

The RAG server agent is no longer maintained in the applications/catalog catalog. The
mesh was extracted to a standalone example, and the RAG server program now lives,
as the single canonical copy, at:

    applications/chatbot-mesh/agents/rag-server/

The catalog copy here was a near-duplicate that had diverged from the example, so
it was removed to end the duplication (GH-511). The example's integrations and
Helm chart consume the canonical copy.

The applications/catalog rel09 mesh specifications are the historical record of the
mesh's development here before extraction; see applications/chatbot-mesh/docs for the
canonical specifications.

This relocation follows the library membership rule: the current RAG server is
mesh-specific composition with one consumer, while reusable corpus behavior
remains in the Knowledge Manager library profiles. A future independently useful
RAG server may be promoted only with a canonical family, SRD, conformance,
parameterization, and portable closure; it must not be copied from the example.
