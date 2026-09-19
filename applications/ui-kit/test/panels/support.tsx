import type { ReactNode } from "react";
import { createKitClient } from "../../src/client/client";
import { KitClientProvider } from "../../src/client/context";
import { fixtureFetch } from "../../src/fixtures";

export class FakeEventSource {
  static last?: FakeEventSource;
  listeners = new Map<string, (event: MessageEvent) => void>();
  onerror: (() => void) | null = null;
  constructor(readonly url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(kind: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(kind, listener);
  }
  close() {}
  emit(kind: string, data: string) {
    this.listeners.get(kind)?.(new MessageEvent(kind, { data }));
  }
}

// FixtureProvider renders panels against the recorded contract fixtures.
export function FixtureProvider({ children, absentAgents = [], overrides }: { children: ReactNode; absentAgents?: string[]; overrides?: Record<string, unknown> }) {
  const client = createKitClient({ fetch: fixtureFetch({ absentAgents, overrides }), EventSource: FakeEventSource as unknown as typeof EventSource });
  return <KitClientProvider client={client}>{children}</KitClientProvider>;
}
