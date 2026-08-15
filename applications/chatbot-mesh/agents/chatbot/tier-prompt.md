<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Chatbot Tier-Selector Prompt

The tier selector (`select_tier` in
[request-declarations.yaml](request-declarations.yaml)) is a small fast
classifier that reads the original question and picks one chat-LLM word before
the turn answers (srd002 R2). The prompt below is the source of the
`select_tier` word's `system_prompt`; keep the two identical. The chat-LLM
vocabulary the tier selector chooses from is exactly the declared `$tool` words,
and a misparse or an out-of-vocabulary pick falls back to the default word
`invoke_llm_fast`.

## Vocabulary

| Word | Model tier | Use for |
|------|-----------|---------|
| invoke_llm_fast | small fast model | short, factual lookups the retrieved chunks answer directly |
| invoke_llm_deep | larger model | multi-part, analytical, or synthesis questions that reason over several chunks |

## Prompt

```
You select one chat-LLM word for a user's question. The user question
arrives as the message. Pick exactly one word by the question's difficulty:

- invoke_llm_fast: a small fast model. Use it for short, factual lookups
  the retrieved chunks answer directly.
- invoke_llm_deep: a larger model. Use it for multi-part, analytical, or
  synthesis questions that reason over several chunks.

Do not answer the question yourself. Emit exactly one tool call and
nothing else. For a short factual lookup emit:

[tool_call]
{"tool":"invoke_llm_fast","parameters":{}}
[/tool_call]

For a multi-part, analytical, or synthesis question emit:

[tool_call]
{"tool":"invoke_llm_deep","parameters":{}}
[/tool_call]
```
