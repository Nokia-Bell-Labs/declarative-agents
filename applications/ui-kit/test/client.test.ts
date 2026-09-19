import { describe, expect, it, vi } from "vitest";
import { createKitClient, HTTPError, proxyPath } from "../src/client/client";

function recordingFetch(status = 200, body: unknown = {}) {
  const calls: string[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    calls.push(String(input));
    return status === 200 ? Response.json(body) : new Response("x", { status });
  }) as unknown as typeof globalThis.fetch;
  return { calls, fetch };
}

describe("kit client", () => {
  it("reads same-origin by default and absolute under a remote base URL (srd004 R6.1)", async () => {
    const same = recordingFetch();
    await createKitClient({ fetch: same.fetch }).getJSON("/monitor/state");
    expect(same.calls).toEqual(["/monitor/state"]);

    const remote = recordingFetch();
    const client = createKitClient({ baseUrl: "http://remote:9/", fetch: remote.fetch });
    await client.getJSON("/monitor/state");
    await client.getProxyJSON("collector", "query/traces");
    expect(remote.calls).toEqual(["http://remote:9/monitor/state", "http://remote:9/monitor-proxy/collector/query/traces"]);
  });

  it("builds every proxy URL through one function (srd004 R2.1)", () => {
    expect(proxyPath("rag0", "monitor/state")).toBe("/monitor-proxy/rag0/monitor/state");
    expect(proxyPath("rag0", "/monitor/state")).toBe("/monitor-proxy/rag0/monitor/state");
  });

  it("reads a proxy 404 as not deployed and other failures as errors (srd004 R2.2)", async () => {
    await expect(createKitClient({ fetch: recordingFetch(404).fetch }).getProxyJSON("rag1", "monitor/state")).resolves.toEqual({ deployed: false });
    await expect(createKitClient({ fetch: recordingFetch(502).fetch }).getProxyJSON("rag1", "monitor/state")).rejects.toBeInstanceOf(HTTPError);
    await expect(createKitClient({ fetch: recordingFetch(200, { run: {} }).fetch }).getProxyJSON("rag1", "monitor/state")).resolves.toEqual({
      deployed: true,
      body: { run: {} },
    });
  });

  it("opens event streams at the based URL", () => {
    const opened: string[] = [];
    class FakeSource {
      constructor(url: string) {
        opened.push(url);
      }
    }
    const client = createKitClient({ baseUrl: "http://remote:9", EventSource: FakeSource as unknown as typeof EventSource });
    client.openEventStream(proxyPath("chatbot", "monitor/events/stream"));
    expect(opened).toEqual(["http://remote:9/monitor-proxy/chatbot/monitor/events/stream"]);
  });
});
