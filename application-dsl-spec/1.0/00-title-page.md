---
title: A Domain-Specific Language for Agents
subtitle: Multi-Agent Systems as Microservices Applications
author: Nokia Bell Labs
date: September 2026
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!--
Publication venue notes (draft positioning; not part of the paper):

Genre. This reads as a software-engineering-of-agents paper (how agent
software is built, structured, and kept correct), not an ML-benchmark paper
and not a pure PL/language-theory paper. Its contribution is a declarative
model + a real system kept honest by machine-checked conformance, with no
quantitative evaluation.

Closest related work (verified on arXiv):
  - StateFlow (2403.11322, COLM 2024): task-solving as a finite state machine.
  - Formal-LLM (2402.00798): automaton-supervised agent plan generation.
  - LMQL / "Prompting Is Programming" (2212.06094, PLDI 2023): a language for
    LLM programs (venue precedent for language framing).
  - CoALA (TMLR 2024): cognitive-architecture framing for language agents.
  - "Externalization in LLM Agents ... Harness Engineering" (2026, cs.SE):
    harness-engineering survey; the nearest motivation anchor.

Candidate venues, in recommended order:
  1. arXiv now - primary cs.SE, cross-list cs.MA. Current shape is ready.
  2. AIware / FORGE (AI foundation models & software engineering): purpose-fit
     for "how to build agent software"; tolerant of design + case-study papers.
  3. ICSE / FSE (SEIP or main track): "SE for AI agents" is in scope; would
     require recasting the Examples section as a case-study evaluation.
  4. COLM: agents-systems audience; expects some empirical/benchmark evidence.
  5. AAMAS (engineering track): multi-agent composition heritage.

Gap to close for a refereed SE venue: sharpen the Examples section into an
explicit evaluation (what declaring the harness catches that ad-hoc code does
not); no new system content required.
-->

We define a YAML-based inner DSL for agents and the multi-agent applications
they compose. In the language an agent is a finite-state machine whose
transitions dispatch tools, and an application is a composition of such agents
together with the messages between them. Because each agent is independently
deployed, addressable, and bound by a declared contract, these applications
deploy as microservices applications. A single machine model spans the range of
agents: from fully deterministic workflow services whose every transition fires
a fixed action, to fully probabilistic LLM-based services that dispatch RAG
retrieval or branch on an agentic tool call. We ground the language in examples
defined in this repository, including a chatbot mesh and a coding agent.
