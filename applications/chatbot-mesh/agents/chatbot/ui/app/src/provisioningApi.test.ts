import { describe, expect, it } from "vitest";

import { applyMesh, fetchMeshState, fetchRollout, PROVISIONING_BASE, type MeshView } from "./provisioningApi";
import { recordingClient } from "./testClient";

const wireState = {
  schema_version: "1",
  rags: [{ name: "rag0", collection: "corpus0", embeddingModel: "embed", replicas: 1 }],
  llmInCluster: true,
  llmExternalURL: "",
  llmChatModel: "chat",
  llmEmbedModel: "embed",
  llmChatModels: ["chat"],
  llmTierModel: "tier",
  llmTopology: "single",
  paramsNResults: 4,
  paramsChunkCap: 0,
  paramsTierDefault: "fast",
};

const view: MeshView = {
  rags: wireState.rags,
  llm: {
    inCluster: true,
    externalURL: "",
    chatModel: "chat",
    embedModel: "embed",
    chatModels: ["chat"],
    tierModel: "tier",
    topology: "single",
  },
  params: { nResults: 4, chunkCap: 0, tierDefault: "fast" },
};

describe("fetchMeshState", () => {
  it("reads the state with the bearer token and reshapes the flat wire fields", async () => {
    const { client, calls } = recordingClient({ [`${PROVISIONING_BASE}/state`]: wireState });
    expect(await fetchMeshState(client, "tok")).toEqual(view);
    expect(calls[0].url).toBe(`${PROVISIONING_BASE}/state`);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].headers.get("authorization")).toBe("Bearer tok");
  });

  it("surfaces the error the intake returns", async () => {
    const { client } = recordingClient({}, { respond: () => Response.json({ error: "token rejected" }, { status: 403 }) });
    await expect(fetchMeshState(client, "tok")).rejects.toThrow("token rejected");
  });
});

describe("applyMesh", () => {
  it("posts the draft as JSON with the bearer token", async () => {
    const { client, calls } = recordingClient({}, { respond: () => new Response(null, { status: 202 }) });
    await applyMesh(client, "tok", view);
    expect(calls[0].url).toBe(`${PROVISIONING_BASE}/apply`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].headers.get("authorization")).toBe("Bearer tok");
    expect(calls[0].headers.get("content-type")).toBe("application/json");
    expect(JSON.parse(calls[0].body ?? "")).toEqual(view);
  });

  it("falls back to the HTTP status when the error body is not JSON", async () => {
    const { client } = recordingClient({}, { respond: () => new Response("upstream down", { status: 502, statusText: "Bad Gateway" }) });
    await expect(applyMesh(client, "tok", view)).rejects.toThrow("502 Bad Gateway");
  });
});

describe("fetchRollout", () => {
  it("reads the rollout poll through the client's base URL", async () => {
    const rollout = { phase: "complete", ready: 1, desired: 1, revision: 3 };
    const { client, calls } = recordingClient({ [`${PROVISIONING_BASE}/rollout`]: rollout }, { baseUrl: "https://mesh.example" });
    expect(await fetchRollout(client, "tok")).toEqual(rollout);
    expect(calls[0].url).toBe(`https://mesh.example${PROVISIONING_BASE}/rollout`);
  });

  it("sends no authorization header without a token", async () => {
    const { client, calls } = recordingClient({ [`${PROVISIONING_BASE}/rollout`]: {} });
    await fetchRollout(client, "");
    expect(calls[0].headers.has("authorization")).toBe(false);
  });
});
