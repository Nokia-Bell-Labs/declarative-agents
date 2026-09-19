import { createKitClient, type KitClient } from "@declarative-agents/ui-kit";
import { fixtureFetch } from "@declarative-agents/ui-kit/fixtures";

// Test-only: a kit client whose fetch answers from the kit fixtures plus the
// given per-path overrides, recording each call's URL, method, headers, and
// body so a test can assert what a domain call sent.
export interface RecordedCall {
  url: string;
  method: string;
  headers: Headers;
  body?: string;
}

export function recordingClient(
  overrides: Record<string, unknown> = {},
  options: { baseUrl?: string; respond?: (url: string) => Response | Promise<Response> | undefined } = {},
): { client: KitClient; calls: RecordedCall[] } {
  const calls: RecordedCall[] = [];
  const fixtures = fixtureFetch({ overrides });
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const url = String(input);
    calls.push({
      url,
      method: init?.method ?? "GET",
      headers: new Headers(init?.headers),
      body: typeof init?.body === "string" ? init.body : undefined,
    });
    return (await options.respond?.(url)) ?? fixtures(input, init);
  };
  return { client: createKitClient({ fetch, baseUrl: options.baseUrl }), calls };
}
