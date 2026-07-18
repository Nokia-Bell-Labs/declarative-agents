import { useRef, useState } from "react";
import { sendChat, type Answer, type Turn } from "./api";

interface Message {
  role: "user" | "assistant";
  content: string;
  sources?: string[];
  grounded?: boolean;
  error?: boolean;
}

function historyFor(messages: Message[]): Turn[] {
  return messages
    .filter((m) => !m.error)
    .map((m) => ({ role: m.role, content: m.content }));
}

function Sources({ sources }: { sources: string[] }) {
  return (
    <div className="sources">
      {sources.map((s) => (
        <span className="source-chip" key={s} title="Retrieved record id">
          {s}
        </span>
      ))}
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
            {msg.sources && msg.sources.length > 0 ? (
              <Sources sources={msg.sources} />
            ) : (
              <span className="degraded" title="No corpus chunks were cited in this answer">
                no sources — ungrounded answer
              </span>
            )}
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

    try {
      const answer: Answer = await sendChat({ message, history: priorHistory });
      setMessages((prev) => [
        ...prev,
        { role: "assistant", content: answer.text, sources: answer.sources, grounded: answer.grounded },
      ]);
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err);
      setMessages((prev) => [
        ...prev,
        { role: "assistant", content: `Request failed: ${detail}`, error: true },
      ]);
    } finally {
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
            Ask a question. The agent embeds it, queries the RAG server, and answers grounded in the
            retrieved corpus chunks, citing the record ids.
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
