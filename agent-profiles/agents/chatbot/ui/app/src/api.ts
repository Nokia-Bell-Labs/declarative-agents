// Client for the chatbot chat endpoint. History is kept client-side (srd014 R4):
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

// The chat machine_request maps its terminal signal to { answer: <text> }. The
// answer text carries the model's inline citations of the retrieved record ids;
// the panel extracts those for display (see extractSources). A structured
// sources/degraded response field is a follow-on that needs the machine_request
// response mapper to read command state, not just the terminal word output.
export interface ChatResponse {
  answer?: string;
  error?: string;
  message?: string;
  trace?: { trace_id?: string };
}

export interface Answer {
  text: string;
  sources: string[];
  grounded: boolean;
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
  const sources = extractSources(text);
  return { text, sources, grounded: sources.length > 0, traceId: body.trace?.trace_id };
}

// The grounding system prompt asks the model to cite the chunk identity (the
// record id) for each claim. Models put those citations in square brackets, so
// we surface the bracketed tokens as source chips. A turn that grounds nothing
// (RAG degraded, or the corpus lacked the answer) yields no citations, which the
// panel renders as the degraded indicator.
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
