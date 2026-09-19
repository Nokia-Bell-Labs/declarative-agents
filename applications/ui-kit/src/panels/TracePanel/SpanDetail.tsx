import { isErrorSpan, type TraceModel, type TraceSpan } from "../../api/traceApi";
import type { Continuation } from "./traceGroups";
import { spanAnswer } from "./traceLayout";

// The span's content (GH-652): identity, timing, the cross-service links, and
// every collected attribute. Captured prompts and answers render as messages
// (GH-658, GH-726); long values fold behind their key.

// The keys most worth reading first.
const CONTENT_KEYS = ["prompt", "input", "output", "request", "response", "message", "body"];
const MESSAGE_KEYS = ["gen_ai.input.messages", "gen_ai.output.messages", "llm.input.delta", "llm.output.delta", "http.request.body", "http.response.body"];

export interface SpanMessage {
  role: string;
  content: string;
}

function flatten(content: unknown): string {
  return Array.isArray(content)
    ? content.map((part) => (part && typeof part === "object" && "text" in part ? String((part as { text: unknown }).text) : "")).join("")
    : String(content ?? "");
}

// parseMessages reads a captured prompt or response as messages, one per
// role; anything it cannot read returns undefined for the generic rendering.
export function parseMessages(value: string): SpanMessage[] | undefined {
  try {
    const parsed = JSON.parse(value);
    // A provider reply envelope: its message is the assistant block, the rest
    // folds behind "response" (GH-728).
    if (parsed?.message && typeof parsed.message === "object" && "content" in parsed.message) {
      const rest = { ...parsed };
      delete rest.message;
      return [
        { role: String(parsed.message.role ?? "assistant"), content: flatten(parsed.message.content) },
        { role: "response", content: JSON.stringify(rest, null, 1) },
      ];
    }
    // An Ollama generate reply carries the text under response.
    if (typeof parsed?.response === "string") {
      const rest = { ...parsed };
      delete rest.response;
      return [
        { role: "assistant", content: parsed.response },
        { role: "response", content: JSON.stringify(rest, null, 1) },
      ];
    }
    // A whole chat request: its messages render as messages and the rest of
    // the body folds behind "request".
    const list = Array.isArray(parsed) ? parsed : Array.isArray(parsed?.messages) ? parsed.messages : [parsed];
    if (list.every((m: unknown) => m && typeof m === "object" && "content" in (m as object))) {
      const messages: SpanMessage[] = list.map((m: { role?: unknown; content?: unknown }) => ({ role: String(m.role ?? "message"), content: flatten(m.content) }));
      if (!Array.isArray(parsed) && Array.isArray(parsed?.messages)) {
        const rest = { ...parsed };
        delete rest.messages;
        messages.push({ role: "request", content: JSON.stringify(rest, null, 1) });
      }
      return messages;
    }
  } catch {
    // fall through to the generic rendering
  }
  return undefined;
}

function prettyJSON(value: string): string | undefined {
  try {
    return JSON.stringify(JSON.parse(value), null, 1);
  } catch {
    return undefined;
  }
}

// messagesFor picks the message rendering of one attribute, or undefined.
function messagesFor(span: TraceSpan, key: string, value: string): SpanMessage[] | undefined {
  if (!MESSAGE_KEYS.includes(key)) return undefined;
  const messages = parseMessages(value);
  if (messages) return messages;
  // A truncated response body no longer parses; the lenient answer
  // extraction still finds the text, with the raw remainder folded.
  if (key === "http.response.body") {
    const answer = spanAnswer(span);
    if (answer !== undefined) {
      return [
        { role: "assistant", content: answer },
        { role: "response (truncated)", content: value },
      ];
    }
  }
  // An embed or a rerank crosses the same boundary with no messages at all;
  // those read as the request and response they are, pretty-printed.
  const payload = prettyJSON(value);
  return payload === undefined ? undefined : [{ role: key.endsWith("request.body") ? "request" : "response", content: payload }];
}

function linkLabel(link: Continuation): string {
  const call = link.span.command ?? link.span.name;
  return link.direction === "sent" ? `→ continues at ${link.span.service}: ${call}` : `← received from ${link.span.service}: ${call}`;
}

export function SpanDetail({
  span,
  model,
  links = [],
  onFollow,
  onClose,
}: {
  span: TraceSpan;
  model: TraceModel;
  links?: Continuation[];
  onFollow?: (span: TraceSpan) => void;
  onClose: () => void;
}) {
  const entries = Object.entries(span.attributes ?? {}).map(([key, value]) => [key, typeof value === "string" ? value : JSON.stringify(value) ?? ""] as const);
  const rank = (key: string) => {
    const index = CONTENT_KEYS.findIndex((content) => key.toLowerCase().includes(content));
    return index === -1 ? CONTENT_KEYS.length : index;
  };
  entries.sort((a, b) => rank(a[0]) - rank(b[0]) || a[0].localeCompare(b[0]));
  return (
    <div className={`span-detail${isErrorSpan(span) ? " span-error" : ""}`} role="dialog" aria-label="Span content" data-testid="span-detail">
      <div className="span-detail-head">
        <span className="span-detail-title">{span.command ?? span.name}</span>
        <span className="trace-svc">{span.service}</span>
        <span className="trace-ms">
          +{((span.startUs - model.startUs) / 1000).toFixed(0)} ms · {(span.durationUs / 1000).toFixed(1)} ms
        </span>
        {span.signal && <span className="walk-signal">{span.signal}</span>}
        {isErrorSpan(span) && <span className="span-error-mark">error</span>}
        {span.target && <span className="timeline-seq-target">→ {span.target}</span>}
        <button type="button" className="trace-overlay-close" aria-label="close span content" onClick={onClose}>
          ✕
        </button>
      </div>
      {isErrorSpan(span) && (
        <div className="span-detail-status" data-testid="span-detail-status">
          Status error{span.status?.description ? `: ${span.status.description}` : ""}
        </div>
      )}
      {links.length > 0 && onFollow && (
        <div className="span-detail-links">
          {links.map((link) => (
            <button key={`${link.direction}:${link.span.id}`} type="button" className="span-follow" onClick={() => onFollow(link.span)}>
              {linkLabel(link)}
            </button>
          ))}
        </div>
      )}
      {span.attributes?.["gen_ai.tool.name"] !== undefined && !span.command?.endsWith("_via_tool") && (
        <div className="span-vocab-note">"tool" below is the machine's word for a step (execute_tool), not an LLM tool call. LLM tool calls are the tool call rows.</div>
      )}
      <dl className="span-detail-attrs">
        {entries.length === 0 && <div className="trace-notice">This span carries no attributes.</div>}
        {entries.map(([key, value]) => {
          const messages = messagesFor(span, key, value);
          if (messages) {
            return (
              <div className="span-messages" key={key}>
                <div className="span-messages-key">{key}</div>
                {messages.map((message, index) => (
                  // The prompt and the answer are the span's story; system
                  // preambles and raw request bodies stay folded.
                  <details key={`${message.role}:${index}`} className="span-message" open={message.role === "user" || message.role === "assistant" || message.role === "response"}>
                    <summary>{message.role}</summary>
                    <pre>{message.content}</pre>
                  </details>
                ))}
              </div>
            );
          }
          return value.length > 120 ? (
            <details key={key} className="span-attr-long">
              <summary>{key}</summary>
              <pre>{value}</pre>
            </details>
          ) : (
            <div className="span-attr" key={key}>
              <dt>{key}</dt>
              <dd>{value}</dd>
            </div>
          );
        })}
      </dl>
    </div>
  );
}
