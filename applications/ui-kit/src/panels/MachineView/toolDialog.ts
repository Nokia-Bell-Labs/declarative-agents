import type { DeclaredTool } from "../../api/monitorApi";

// What a word's declaration says, arranged for a reader who clicked its tag on
// the figure. Everything here is read from the declaration the agent serves
// (GET /monitor/tools/declared, srd033 R9): what kind of step it is, what it
// calls, what it reads, and the prompts it carries. Nothing is inferred from a
// hostname -- the REST line is the declared side effect. Base is
// agentic-wiki-mesh's toolDialog; cohere-demo's copy contributed the OTLP
// receiver lifecycle kinds, the model endpoint, and the raw record.

export interface DialogParameter {
  name: string;
  type: string;
  required: boolean;
}

export interface DialogPrompt {
  label: string;
  text: string;
}

export interface DialogView {
  kind: string;
  kindClass: string;
  // What the word calls out to, from its declared side effects.
  calls: string[];
  // provider and model for a word the runtime dispatches at a model, and the
  // endpoint it calls when the declaration names one.
  model?: string;
  description?: string;
  problem?: string;
  goals: string[];
  emits: string[];
  parameters: DialogParameter[];
  // The labels this word reads from command state, from its $from(...) selectors.
  inputs: string[];
  prompts: DialogPrompt[];
  // The declaration as served, for a reader who wants every field.
  raw: string;
}

const KIND_FOR_INIT: Record<string, [string, string]> = {
  rest_client_invoke: ["External REST call", "rest"],
  rest_client_get: ["External REST call", "rest"],
  invoke_llm: ["LLM call", "llm"],
  rest_server_launch: ["Lifecycle", "lifecycle"],
  rest_server_stop: ["Lifecycle", "lifecycle"],
  rest_await_event: ["Lifecycle", "lifecycle"],
  otlp_receiver_launch: ["Lifecycle", "lifecycle"],
  otlp_receiver_stop: ["Lifecycle", "lifecycle"],
  exit_agent: ["Lifecycle", "lifecycle"],
  file_read: ["Local file", "process"],
  file_write: ["Local file", "process"],
  file_edit: ["Local file", "process"],
  exec: ["Local process", "process"],
};

const PROMPT_LABELS: Record<string, string> = {
  system_prompt: "system prompt",
  template: "template",
  item_template: "item template",
  user_prompt_from: "reads the prompt from",
};

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function asStrings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === "string") : [];
}

// resolveEnvDefault reads the default out of an environment reference, so a
// dialog shows the model the deployment runs rather than the reference's text.
export function resolveEnvDefault(value: string): string {
  const match = /^\$\{[A-Z0-9_]+:-(.*)\}$/.exec(value.trim());
  return match ? match[1] : value;
}

// The selectors a word reads from command state, in declaration order.
function declaredInputs(config: Record<string, unknown>): string[] {
  const found: string[] = [];
  const walk = (value: unknown) => {
    if (typeof value === "string") {
      for (const match of value.matchAll(/\$from\(([^)]+)\)/g)) {
        if (!found.includes(match[1])) found.push(match[1]);
      }
    } else if (Array.isArray(value)) {
      value.forEach(walk);
    } else if (value && typeof value === "object") {
      Object.values(value).forEach(walk);
    }
  };
  walk(config);
  return found;
}

export function dialogView(declaration: DeclaredTool): DialogView {
  const config = asRecord(declaration.config);
  const [kind, kindClass] = KIND_FOR_INIT[String(declaration.init ?? "")] ?? ["Internal step", "internal"];

  const calls: string[] = [];
  for (const effect of Array.isArray(declaration.side_effects) ? declaration.side_effects : []) {
    const target = asRecord(effect).target;
    if (typeof target === "string" && !calls.includes(target)) calls.push(target);
  }
  // A word whose operation the agent's own definition names, with no side
  // effect declared beside it, still says which client and operation it runs.
  if (calls.length === 0 && typeof config.rest_ref === "string" && typeof config.operation === "string") {
    calls.push(`${config.rest_ref}.${config.operation}`);
  }

  const provider = typeof config.provider === "string" ? resolveEnvDefault(config.provider) : undefined;
  const model = typeof config.model === "string" ? resolveEnvDefault(config.model) : undefined;
  const endpoint = typeof config.provider_url === "string" ? resolveEnvDefault(config.provider_url) : undefined;
  const modelLine = provider || model ? [provider, model].filter(Boolean).join(" · ") + (endpoint ? ` at ${endpoint}` : "") : undefined;

  const parameters: DialogParameter[] = [];
  const properties = asRecord(asRecord(declaration.parameters).properties);
  const required = new Set(asStrings(asRecord(declaration.parameters).required));
  for (const [name, schema] of Object.entries(properties)) {
    const type = asRecord(schema).type;
    parameters.push({ name, type: typeof type === "string" ? type : "any", required: required.has(name) });
  }

  const prompts: DialogPrompt[] = [];
  for (const [field, label] of Object.entries(PROMPT_LABELS)) {
    const text = config[field];
    if (typeof text === "string" && text.trim() !== "") prompts.push({ label, text });
  }

  return {
    kind,
    kindClass,
    calls,
    model: modelLine,
    description: typeof declaration.description === "string" ? declaration.description : undefined,
    problem: typeof declaration.problem === "string" ? declaration.problem : undefined,
    goals: asStrings(declaration.goals),
    emits: asStrings(declaration.emits),
    parameters,
    inputs: declaredInputs(config),
    prompts,
    raw: JSON.stringify(declaration, null, 2),
  };
}
