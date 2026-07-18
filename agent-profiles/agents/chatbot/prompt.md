# Chatbot grounding prompt

The chatbot answers a user's question grounded in chunks retrieved from a RAG
server. Each turn embeds the message once, queries the RAG server with the
vector, composes a grounding prompt from the original message and the retrieved
chunks (through command-state `$from()` addressing), and invokes the chat model.

The model answers only from the retrieved chunks, cites each chunk's record id,
and says so plainly when the chunks do not contain the answer. The active prompt
is configured on the `invoke_llm` word in `request-declarations.yaml`.
