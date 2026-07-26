import { createContext, useContext, useMemo, useRef, useState, type ReactNode } from "react";

// A turn records the time window of one chat request. Monitor events do not carry
// the request id (the monitor RunEvent has no request_id field), so the panel
// correlates a turn to the mesh's events by time window: events observed between a
// turn's start and end are its events. Request-id-tagged correlation is a follow-on
// that needs the run events to carry the request id.
export interface Turn {
  id: number;
  message: string;
  startedAt: number;
  endedAt?: number;
  traceId?: string;
}

interface TurnStore {
  turns: Turn[];
  selectedId: number | null;
  select: (id: number | null) => void;
  startTurn: (message: string) => number;
  endTurn: (id: number, traceId?: string) => void;
}

const TurnContext = createContext<TurnStore | null>(null);

export function TurnProvider({ children }: { children: ReactNode }) {
  const [turns, setTurns] = useState<Turn[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const nextId = useRef(0);

  const store = useMemo<TurnStore>(
    () => ({
      turns,
      selectedId,
      select: setSelectedId,
      startTurn: (message: string) => {
        const id = nextId.current++;
        setTurns((prev) => [{ id, message, startedAt: Date.now() }, ...prev].slice(0, 50));
        return id;
      },
      endTurn: (id: number, traceId?: string) => {
        setTurns((prev) => prev.map((t) => (t.id === id ? { ...t, endedAt: Date.now(), traceId } : t)));
      },
    }),
    [turns, selectedId],
  );

  return <TurnContext.Provider value={store}>{children}</TurnContext.Provider>;
}

export function useTurns(): TurnStore {
  const ctx = useContext(TurnContext);
  if (!ctx) throw new Error("useTurns must be used within a TurnProvider");
  return ctx;
}

// eventInSelectedTurn reports whether an event timestamp falls in the selected
// turn's window (start .. end, or start .. now for an in-flight turn).
export function eventInWindow(turn: Turn | undefined, at: number): boolean {
  if (!turn) return false;
  const end = turn.endedAt ?? Date.now();
  return at >= turn.startedAt && at <= end + 500;
}
