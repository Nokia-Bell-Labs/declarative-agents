// @vitest-environment jsdom
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { createKitClient } from "../src/client/client";
import { KitClientProvider } from "../src/client/context";
import { fixtureFetch, fixtures } from "../src/fixtures";
import { useAgentMonitor } from "../src/hooks/useAgentMonitor";
import { useTrace, useTraceList } from "../src/hooks/useTrace";

class FakeEventSource {
  static last?: FakeEventSource;
  listeners = new Map<string, (event: MessageEvent) => void>();
  closed = false;
  onerror: (() => void) | null = null;
  constructor(readonly url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(kind: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(kind, listener);
  }
  close() {
    this.closed = true;
  }
  emit(kind: string, data: string) {
    this.listeners.get(kind)?.(new MessageEvent(kind, { data }));
  }
}

function wrapper(absentAgents: string[] = []) {
  const client = createKitClient({ fetch: fixtureFetch({ absentAgents }), EventSource: FakeEventSource as unknown as typeof EventSource });
  return ({ children }: { children: ReactNode }) => <KitClientProvider client={client}>{children}</KitClientProvider>;
}

describe("useAgentMonitor", () => {
  it("connects, opens the stream, and buffers run events", async () => {
    const { result } = renderHook(() => useAgentMonitor("chatbot"), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.status).toBe("connected"));
    expect(result.current.run).toEqual(fixtures["/monitor/state"].run);
    const stream = FakeEventSource.last!;
    expect(stream.url).toBe("/monitor-proxy/chatbot/monitor/events/stream");
    act(() => {
      for (const frame of fixtures["/monitor/events/stream"]) stream.emit(frame.event, frame.data);
    });
    expect(result.current.runEvents).toHaveLength(1);
    expect(result.current.metricCount).toBe(1);
  });

  it("reports an agent the proxy does not deploy as absent", async () => {
    const { result } = renderHook(() => useAgentMonitor("rag1"), { wrapper: wrapper(["rag1"]) });
    await waitFor(() => expect(result.current.status).toBe("absent"));
  });
});

describe("trace hooks", () => {
  it("reads a trace and a page from the declared backend", async () => {
    const trace = renderHook(() => useTrace("collector", "t1"), { wrapper: wrapper() });
    await waitFor(() => expect(trace.result.current.status).toBe("ok"));
    const idle = renderHook(() => useTrace("collector", undefined), { wrapper: wrapper() });
    expect(idle.result.current.status).toBe("idle");
    const list = renderHook(() => useTraceList("collector", 50, 0), { wrapper: wrapper() });
    await waitFor(() => expect(list.result.current.status).toBe("ok"));
  });
});
