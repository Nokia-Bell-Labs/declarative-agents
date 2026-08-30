import { useRef, useState } from "react";
import {
  sendChat,
  type Answer,
  type Attribution,
  type AttributionReason,
  type ComposedSource,
  type Turn,
} from "./api";
import { useTurns } from "./turns";

interface Message {
  role: "user" | "assistant";
  content: string;
  attribution?: Attribution;
  citations?: string[];
  error?: boolean;
}

function historyFor(messages: Message[]): Turn[] {
  return messages
    .filter((m) => !m.error)
    .map((m) => ({ role: m.role, content: m.content }));
}

// Composed sources are what the server retrieved and fed to the answer prompt,
// reported per source with the number of chunks it contributed. A source that
// composed zero documents is shown too: it took part in the turn and
// contributed nothing, which is worth seeing rather than hiding.
function ComposedSources({ sources }: { sources: ComposedSource[] }) {
  return (
    <div className="sources">
      {sources.map((source) => (
        <span
          className="source-chip"
          key={source.name}
          title="RAG source that took part in composing this answer"
        >
          {source.name} · {source.documents} {source.documents === 1 ? "document" : "documents"}
        </span>
      ))}
    </div>
  );
}

// Inline citations are what the model wrote in its answer text. They are not
// evidence of retrieval -- small chat models omit them from answers that did
// compose from retrieved chunks -- so they render as their own row, distinct
// from the composed sources above.
function Citations({ citations }: { citations: string[] }) {
  return (
    <div className="citations">
      {citations.map((citation) => (
        <span
          className="citation-chip"
          key={citation}
          title="Citation the model wrote inline in its answer"
        >
          {citation}
        </span>
      ))}
    </div>
  );
}

function describeReason(reason: AttributionReason): string {
  const named = "sources" in reason ? reason.sources.join(", ") : "";
  switch (reason.kind) {
    case "none-composed":
      return "no source took part in composing this answer";
    case "no-documents":
      return `${named} composed but retrieved no documents, which is what an unseeded corpus looks like`;
    case "query-failed":
      return `query failed: ${named}`;
    case "embedding-model-excluded":
      return `excluded for an embedding-model mismatch: ${named}`;
    case "not-selected":
      return `not selected by the source router: ${named}`;
  }
}

// An unattributed turn says which source outcome explains it. Collapsing every
// cause into one sentence is what made the panel disagree with the mesh.
function Unattributed({ reasons }: { reasons: AttributionReason[] }) {
  return (
    <div className="unattributed">
      <span className="degraded" title="No retrieved document reached this answer">
        unattributed answer
      </span>
      <ul className="reason-list">
        {reasons.map((reason) => (
          <li key={reason.kind}>{describeReason(reason)}</li>
        ))}
      </ul>
    </div>
  );
}

function MessageBubble({ msg }: { msg: Message }) {
  const cls = msg.error
    ? "bubble bubble-error"
    : msg.role === "user"
      ? "bubble bubble-user"
      : "bubble bubble-assistant";
  return (
    <div className={`bubble-row bubble-row-${msg.role}`}>
      <div className={cls}>
        <div className="bubble-text">{msg.content}</div>
        {msg.role === "assistant" && !msg.error && (
          <div className="bubble-meta">
            {msg.attribution?.attributed ? (
              <ComposedSources sources={msg.attribution.composed} />
            ) : (
              <Unattributed reasons={msg.attribution?.reasons ?? [{ kind: "none-composed" }]} />
            )}
            {msg.citations && msg.citations.length > 0 && <Citations citations={msg.citations} />}
          </div>
        )}
      </div>
    </div>
  );
}

export default function ChatPanel() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);
  const { startTurn, endTurn } = useTurns();

  function scrollToEnd() {
    requestAnimationFrame(() => {
      const el = listRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    });
  }

  async function submit() {
    const message = input.trim();
    if (!message || busy) return;
    const priorHistory = historyFor(messages);
    const userMsg: Message = { role: "user", content: message };
    setMessages((prev) => [...prev, userMsg]);
    setInput("");
    setBusy(true);
    scrollToEnd();

    // Record the turn window so the observability panel can correlate the mesh's
    // events to this turn.
    const turnId = startTurn(message);
    let traceId: string | undefined;
    try {
      const answer: Answer = await sendChat({ message, history: priorHistory });
      traceId = answer.traceId;
      setMessages((prev) => [
        ...prev,
        {
          role: "assistant",
          content: answer.text,
          attribution: answer.attribution,
          citations: answer.citations,
        },
      ]);
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err);
      setMessages((prev) => [
        ...prev,
        { role: "assistant", content: `Request failed: ${detail}`, error: true },
      ]);
    } finally {
      endTurn(turnId, traceId);
      setBusy(false);
      scrollToEnd();
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void submit();
    }
  }

  return (
    <div className="chat">
      <div className="chat-list" ref={listRef}>
        {messages.length === 0 ? (
          <div className="empty">
            Ask a question. The agent embeds it, queries the RAG server, and composes the answer
            from the retrieved corpus chunks. Each answer reports the sources it composed from.
          </div>
        ) : (
          messages.map((m, i) => <MessageBubble msg={m} key={i} />)
        )}
        {busy && (
          <div className="bubble-row bubble-row-assistant">
            <div className="bubble bubble-assistant bubble-pending">…</div>
          </div>
        )}
      </div>
      <div className="chat-input">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Message the chatbot…  (Enter to send, Shift+Enter for a newline)"
          rows={2}
          disabled={busy}
        />
        <button onClick={() => void submit()} disabled={busy || input.trim().length === 0}>
          Send
        </button>
      </div>
    </div>
  );
}
