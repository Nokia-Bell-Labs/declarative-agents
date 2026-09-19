import { MONITOR_PROXY_PREFIX } from "../contract";

// The one place kit code performs I/O (srd004 R6.1). Everything else receives a
// KitClient, so the same panel runs same-origin or against a remote base URL.

export class HTTPError extends Error {
  constructor(
    readonly url: string,
    readonly status: number,
  ) {
    super(`${url} -> HTTP ${status}`);
  }
}

// ProxyResult is a cross-agent read: a proxy 404 means the agent is not
// deployed, which is an answer rather than a failure (srd004 R2.2).
export type ProxyResult<T> = { deployed: false } | { deployed: true; body: T };

export interface KitClientOptions {
  // Prefix for every request; "" (the default) is same-origin.
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
  EventSource?: typeof globalThis.EventSource;
}

export interface KitClient {
  readonly baseUrl: string;
  url(path: string): string;
  // init carries the method, headers, and body of a domain call, such as a
  // chat POST or an authenticated provisioning read.
  request(path: string, init?: RequestInit): Promise<Response>;
  getJSON<T>(path: string, init?: RequestInit): Promise<T>;
  getProxyJSON<T>(agent: string, path: string): Promise<ProxyResult<T>>;
  openEventStream(path: string): EventSource;
}

// proxyPath addresses a contract path on another agent through the serving
// origin's monitor proxy. It is the only function that spells the proxy route.
export function proxyPath(agent: string, path: string): string {
  return `${MONITOR_PROXY_PREFIX}/${encodeURIComponent(agent)}/${path.replace(/^\/+/, "")}`;
}

export function createKitClient(options: KitClientOptions = {}): KitClient {
  const baseUrl = (options.baseUrl ?? "").replace(/\/+$/, "");
  const doFetch = options.fetch ?? ((input, init) => globalThis.fetch(input, init));
  const Source = options.EventSource ?? globalThis.EventSource;

  const url = (path: string) => baseUrl + (path.startsWith("/") ? path : `/${path}`);
  const request = (path: string, init?: RequestInit) => doFetch(url(path), init);
  const getJSON = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const res = await request(path, init);
    if (!res.ok) throw new HTTPError(url(path), res.status);
    return (await res.json()) as T;
  };

  return {
    baseUrl,
    url,
    request,
    getJSON,
    async getProxyJSON<T>(agent: string, path: string): Promise<ProxyResult<T>> {
      const target = proxyPath(agent, path);
      const res = await request(target);
      if (res.status === 404) return { deployed: false };
      if (!res.ok) throw new HTTPError(url(target), res.status);
      return { deployed: true, body: (await res.json()) as T };
    },
    openEventStream(path: string): EventSource {
      if (!Source) throw new Error("EventSource is not available; pass one to createKitClient");
      return new Source(url(path));
    },
  };
}
