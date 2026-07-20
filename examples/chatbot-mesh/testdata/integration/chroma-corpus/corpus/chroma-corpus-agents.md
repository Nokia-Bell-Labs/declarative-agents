# Chroma Corpus Agents

The Chroma corpus agents are two configured profiles that use a local Chroma
vector database as a retrieval boundary. The ingest profile loads repository
documents into a Chroma collection. The reader profile answers a question
grounded in the chunks retrieved from that collection.

Chroma does not embed text server-side over its raw HTTP API, so the agents
compute embeddings at a local Ollama provider and thread the resulting vector
into the Chroma add and query words. No embedding vector is authored by a
model; the vector always rides between boundary words through the REST client
previous-Result parameter threading.
