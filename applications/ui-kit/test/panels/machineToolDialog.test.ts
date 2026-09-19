import { describe, expect, it } from "vitest";

import { dialogView, resolveEnvDefault } from "../../src/panels/MachineView";

// Ported from agentic-wiki-mesh toolDialog.test.ts, plus the cohere-demo
// behaviour merged into the kit (OTLP lifecycle kinds, model endpoint, raw
// record). The declarations below are the shipped ones, trimmed to the fields a dialog
// reads: agents/chatbot/request-provider-declarations.yaml for the Cohere
// words, request-declarations.yaml for the classifier, and
// request-fanout-declarations.yaml for the plain word.

describe("what a word's declaration says", () => {
  it("names the REST call a boundary word makes, from its declared side effect", () => {
    const view = dialogView({
      name: "invoke_cohere_command",
      init: "rest_client_invoke",
      category: "boundary",
      description: "Compose the answer at the Cohere provider over the composed chunks as documents.",
      emits: ["AnswerComposed", "ProviderUnauthorized", "CommandError"],
      config: { rest_ref: "cohere", operation: "compose_grounded" },
      side_effects: [{ kind: "external_api", target: "cohere.chat" }],
    });
    expect(view.kind).toBe("External REST call");
    expect(view.kindClass).toBe("rest");
    expect(view.calls).toEqual(["cohere.chat"]);
    expect(view.emits).toContain("AnswerComposed");
    // No hostname anywhere: the declared target is the whole story.
    expect(JSON.stringify(view)).not.toContain("api.cohere.com");
  });

  it("falls back to the client and operation when no side effect is declared", () => {
    const view = dialogView({ name: "knowledge_query", init: "rest_client_invoke", config: { rest_ref: "knowledge", operation: "query" } });
    expect(view.calls).toEqual(["knowledge.query"]);
  });

  it("names the model of a word the runtime dispatches at a model", () => {
    const view = dialogView({
      name: "select_sources",
      init: "invoke_llm",
      description: "Select declared knowledge source names for the original user question.",
      config: {
        dialect: "/opt/providers/chat-dialect.yaml",
        model: "${CHATBOT_TIER_MODEL:-qwen2.5:3b}",
        system_prompt: "You select which declared knowledge sources should be queried.",
        user_prompt_from: "compose_source_selection_prompt",
      },
    });
    expect(view.kind).toBe("LLM call");
    expect(view.kindClass).toBe("llm");
    // The deployment's model, not the reference's text. The word reads the
    // bound library's dialect, whose path names no provider, so the model
    // stands alone; the turn's bindings name the provider.
    expect(view.model).toBe("qwen2.5:3b");
    expect(view.prompts.map((prompt) => prompt.label)).toEqual(["system prompt", "reads the prompt from"]);
    expect(view.prompts[0].text).toContain("declared knowledge sources");
  });

  it("names a provider a word still declares beside its model", () => {
    const view = dialogView({ name: "legacy", init: "invoke_llm", config: { provider: "${P:-ollama}", model: "m" } });
    expect(view.model).toBe("ollama · m");
  });

  it("reads a plain word as an internal step with nothing to call", () => {
    const view = dialogView({
      name: "capture_request",
      init: "compose",
      category: "word",
      description: "Publish the seeded message under the request label.",
      config: { template: "{\"input\": \"$from(seed).message\"}" },
    });
    expect(view.kind).toBe("Internal step");
    expect(view.calls).toEqual([]);
    expect(view.model).toBeUndefined();
    // What it reads from command state, from its own selectors.
    expect(view.inputs).toEqual(["seed"]);
  });

  it("reads the parameters a word declares and which are required", () => {
    const view = dialogView({
      name: "knowledge_query",
      init: "rest_client_invoke",
      parameters: { type: "object", properties: { query: { type: "string" }, top_k: { type: "integer" } }, required: ["query"] },
    });
    expect(view.parameters).toEqual([
      { name: "query", type: "string", required: true },
      { name: "top_k", type: "integer", required: false },
    ]);
  });

  it("badges the lifecycle and local-process words as what they are", () => {
    expect(dialogView({ name: "launch_chat_requests", init: "rest_server_launch" }).kindClass).toBe("lifecycle");
    expect(dialogView({ name: "write_page", init: "file_write" }).kindClass).toBe("process");
  });

  it("resolves an environment reference to the default it carries", () => {
    expect(resolveEnvDefault("${COHERE_BASE_URL:-https://api.cohere.com}")).toBe("https://api.cohere.com");
    expect(resolveEnvDefault("qwen2.5:3b")).toBe("qwen2.5:3b");
  });

  it("badges the OTLP receiver lifecycle words and names a model's endpoint", () => {
    expect(dialogView({ name: "start_receiver", init: "otlp_receiver_launch" }).kind).toBe("Lifecycle");
    expect(dialogView({ name: "stop_receiver", init: "otlp_receiver_stop" }).kindClass).toBe("lifecycle");
    const view = dialogView({
      name: "compose",
      init: "invoke_llm",
      config: { provider: "cohere", model: "command-r7b-12-2024", provider_url: "${COHERE_BASE_URL:-https://api.cohere.com}" },
    });
    expect(view.model).toBe("cohere · command-r7b-12-2024 at https://api.cohere.com");
  });

  it("keeps the declaration as served, unknown fields included", () => {
    const view = dialogView({ name: "embed", init: "compose", custom_note: "unknown fields ride through" });
    expect(JSON.parse(view.raw).custom_note).toBe("unknown fields ride through");
  });
});
