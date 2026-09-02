// Client for the chatbot chat endpoint. History is kept client-side (srd002 R4):
// the browser sends the accumulated turns with each request and the agent keeps
// no server-side session.

export const CHAT_ENDPOINT = "/api/v1/chat";

export type Role = "user" | "assistant";

export interface Turn {
  role: Role;
  content: string;
}

export interface ChatRequest {
  message: string;
  history: Turn[];
}

// One RAG source's part in a turn. The mesh reports every source it consulted
// under metadata.sources, sorted into four lists by what became of it, and a
// composed entry carries the chunks that source contributed to the answer
// prompt.
//
// The mesh projects these entries before answering (GH-216). They are rendered
// from fan-out join entries that also carry the topology entry the query was
// dispatched to, base_url included, and those addresses used to arrive here.
export interface SourceOutcome {
  name?: string;
  signal?: string;
  documents?: string[][];
  // Reported on an excluded entry only: the identity that did not match the
  // query embedding model, which is why the source was excluded.
  embedding_model?: string;
}

// The chat machine_request maps its terminal signal to an answer and the source
// metadata behind it (srd002 R3, R4). The four source lists are the server's
// own account of the turn: composed sources reached the answer prompt, and the
// other three say why the rest did not.
export interface ChatResponse {
  answer?: string;
  error?: string;
  message?: string;
  trace?: { trace_id?: string };
  metadata?: {
    sources?: {
      composed?: SourceOutcome[];
      embedding_model_excluded?: SourceOutcome[];
      query_failed?: SourceOutcome[];
      not_selected?: string[];
    };
  };
}

export interface ComposedSource {
  name: string;
  documents: number;
}

// Why a turn carries no attribution. Each kind names the sources it concerns,
// so the panel can say which source failed rather than reporting that something
// did.
export type AttributionReason =
  | { kind: "none-composed" }
  | { kind: "no-documents"; sources: string[] }
  | { kind: "query-failed"; sources: string[] }
  | { kind: "embedding-model-excluded"; sources: string[] }
  | { kind: "not-selected"; sources: string[] };

export interface Attribution {
  composed: ComposedSource[];
  attributed: boolean;
  reasons: AttributionReason[];
}

export interface Answer {
  text: string;
  // Sources the server composed the answer from.
  sources: string[];
  // Bracket tokens the model wrote inline, which are not the same thing.
  citations: string[];
  grounded: boolean;
  attribution: Attribution;
  traceId?: string;
}

export class ChatError extends Error {}

export async function sendChat(req: ChatRequest, signal?: AbortSignal): Promise<Answer> {
  let res: Response;
  try {
    res = await fetch(CHAT_ENDPOINT, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(req),
      signal,
    });
  } catch (err) {
    throw new ChatError(err instanceof Error ? err.message : String(err));
  }

  let body: ChatResponse = {};
  try {
    body = (await res.json()) as ChatResponse;
  } catch {
    throw new ChatError(`chat endpoint returned HTTP ${res.status} with a non-JSON body`);
  }

  if (!res.ok) {
    throw new ChatError(body.message ?? body.error ?? `chat endpoint returned HTTP ${res.status}`);
  }

  const text = (body.answer ?? "").trim();
  const attribution = deriveAttribution(body);
  return {
    text,
    sources: attribution.composed.map((source) => source.name),
    citations: extractSources(text),
    grounded: attribution.attributed,
    attribution,
    traceId: body.trace?.trace_id,
  };
}

// deriveAttribution decides whether a turn answered from the corpus, and when it
// did not, which source outcome explains that.
//
// A composed entry means the source took part in composition, not that it had
// anything to contribute: a rag-server bound to an empty collection answers the
// query and composes zero chunks. So attribution requires composed sources that
// carry documents, not merely composed sources.
//
// This is the same rule the integration:chatbot gate applies in
// magefiles/integration_chatbot.go, deliberately: the panel and the gate must
// never disagree about a turn. Change one and the other has to change with it.
export function deriveAttribution(body: ChatResponse): Attribution {
  const sources = body.metadata?.sources ?? {};
  const composed = (sources.composed ?? []).map((outcome, index) => ({
    name: outcomeName(outcome, index),
    documents: documentCount(outcome),
  }));
  const retrieved = composed.reduce((total, source) => total + source.documents, 0);
  if (retrieved > 0) {
    return { composed, attributed: true, reasons: [] };
  }

  const reasons: AttributionReason[] = [];
  if (composed.length === 0) {
    reasons.push({ kind: "none-composed" });
  } else {
    reasons.push({ kind: "no-documents", sources: composed.map((source) => source.name) });
  }
  const failed = outcomeNames(sources.query_failed);
  if (failed.length > 0) {
    reasons.push({ kind: "query-failed", sources: failed });
  }
  const excluded = outcomeNames(sources.embedding_model_excluded);
  if (excluded.length > 0) {
    reasons.push({ kind: "embedding-model-excluded", sources: excluded });
  }
  const unselected = (sources.not_selected ?? []).filter((name) => name.trim() !== "");
  if (unselected.length > 0) {
    reasons.push({ kind: "not-selected", sources: unselected });
  }
  return { composed, attributed: false, reasons };
}

function outcomeName(outcome: SourceOutcome, index: number): string {
  const name = outcome.name?.trim();
  return name ? name : `source ${index}`;
}

function outcomeNames(outcomes: SourceOutcome[] | undefined): string[] {
  return (outcomes ?? [])
    .map((outcome) => outcome.name?.trim() ?? "")
    .filter((name) => name !== "");
}

function documentCount(outcome: SourceOutcome): number {
  const documents = outcome.documents ?? [];
  return documents.reduce((total, query) => total + query.length, 0);
}

// The grounding system prompt asks the model to cite the chunk identity for each
// claim, and models put those citations in square brackets. These are worth
// displaying as what the model wrote, but they do not decide whether a turn is
// attributed: small chat models omit them from answers that did compose from
// retrieved chunks, which is why grounding reads metadata.sources instead.
export function extractSources(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const re = /\[([^\]\n]{1,80})\]/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    const token = m[1].trim();
    if (!token || seen.has(token)) continue;
    seen.add(token);
    out.push(token);
  }
  return out;
}
