import { useCallback, useEffect, useRef, useState } from "react";

// The provisioning surface the panel drives (srd003 R4). It is same-origin at
// /provisioning, routed by the chatbot ingress to the coordinator's intent
// server, so the panel's POST apply avoids the GET-only monitor_proxy and no call
// crosses origin. The base is values-driven via ux.yaml and defaults to the
// same-origin path.
//
// The panel does not reach the deployment API. Its apply is an operator
// desired-state decision (srd004 R3.1) submitted to the coordinator, which
// orchestrates the creator, which alone calls the executor's apply surface
// (srd003 R4.4, srd006 R4.1). Pointing the panel at the executor directly is what
// GH-502 fixed: the executor NetworkPolicy admits only creator-labelled pods, so
// that route was unreachable wherever policy is enforced. Provisioning changes
// deployment values and triggers rollouts only (R4.2).
export const PROVISIONING_BASE =
  (globalThis as { PROVISIONING_API?: string }).PROVISIONING_API ?? "/provisioning/api";

// MeshView mirrors the executor values-plane view: RAG topology, the LLM
// endpoint, and the interesting parameters. There is no per-agent runtime endpoint
// field, so the panel cannot submit transport authority to a running agent.
export interface RagView {
  name: string;
  collection: string;
  embeddingModel: string;
  replicas: number;
  status?: string;
}

export interface LLMView {
  inCluster: boolean;
  externalURL: string;
  chatModel: string;
  embedModel: string;
  chatModels?: string[];
  routerModel?: string;
  topology?: string;
}

export interface ParamsView {
  nResults: number;
  chunkCap: number;
  routerDefault: string;
}

export interface MeshView {
  rags: RagView[];
  llm: LLMView;
  params: ParamsView;
}

// RolloutStatus is what the coordinator's rollout poll serves, field for field:
// the phase the executor reads from kubectl and the counts it reads off the
// Deployment, carried through the creator (GH-686). There is no "unknown" phase
// and no message -- a read the mesh cannot serve answers 502 at every hop rather
// than a successful unknown, so fetchRollout throws instead of resolving.
export interface RolloutStatus {
  phase: "progressing" | "complete";
  ready: number;
  desired: number;
  revision: number;
}

// MeshStateResponse is the flat wire shape GET /provisioning/api/state serves
// (srd006 deployment_api_contract). machine_request response bodies map one
// selector per named field, so the executor -> creator -> coordinator chain
// cannot assemble a nested llm/params object from several source paths in one
// step (GH-753); fetchMeshState reassembles the MeshView the rest of the panel
// renders from these flat fields -- the one reshape in this feature that
// happens client-side rather than in a declarative binding.
interface MeshStateResponse {
  schema_version: string;
  rags: RagView[];
  llmInCluster: boolean;
  llmExternalURL: string;
  llmChatModel: string;
  llmEmbedModel: string;
  llmChatModels: string[];
  llmRouterModel: string;
  llmTopology: string;
  paramsNResults: number;
  paramsChunkCap: number;
  paramsRouterDefault: string;
}

function toMeshView(wire: MeshStateResponse): MeshView {
  return {
    rags: wire.rags,
    llm: {
      inCluster: wire.llmInCluster,
      externalURL: wire.llmExternalURL,
      chatModel: wire.llmChatModel,
      embedModel: wire.llmEmbedModel,
      chatModels: wire.llmChatModels,
      routerModel: wire.llmRouterModel,
      topology: wire.llmTopology,
    },
    params: {
      nResults: wire.paramsNResults,
      chunkCap: wire.paramsChunkCap,
      routerDefault: wire.paramsRouterDefault,
    },
  };
}

function authHeaders(token: string): HeadersInit {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function readError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) return body.error;
  } catch {
    /* fall through to status text */
  }
  return `${res.status} ${res.statusText}`;
}

export async function fetchMeshState(token: string): Promise<MeshView> {
  const res = await fetch(`${PROVISIONING_BASE}/state`, { headers: authHeaders(token) });
  if (!res.ok) throw new Error(await readError(res));
  return toMeshView((await res.json()) as MeshStateResponse);
}

export async function applyMesh(token: string, view: MeshView): Promise<void> {
  const res = await fetch(`${PROVISIONING_BASE}/apply`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(view),
  });
  if (!res.ok) throw new Error(await readError(res));
}

export async function fetchRollout(token: string): Promise<RolloutStatus> {
  const res = await fetch(`${PROVISIONING_BASE}/rollout`, { headers: authHeaders(token) });
  if (!res.ok) throw new Error(await readError(res));
  return (await res.json()) as RolloutStatus;
}

// diffMesh summarizes the edits between the deployed view and the panel's draft, so
// the operator sees exactly what an apply will change before submitting it.
export function diffMesh(current: MeshView, draft: MeshView): string[] {
  const lines: string[] = [];
  const byName = (rs: RagView[]) => new Map(rs.map((r) => [r.name, r]));
  const cur = byName(current.rags);
  const next = byName(draft.rags);
  for (const [name, r] of next) {
    const before = cur.get(name);
    if (!before) {
      lines.push(`add RAG ${name} (collection ${r.collection})`);
    } else if (before.collection !== r.collection || before.embeddingModel !== r.embeddingModel || before.replicas !== r.replicas) {
      lines.push(`change RAG ${name}: collection ${before.collection}→${r.collection}, replicas ${before.replicas}→${r.replicas}`);
    }
  }
  for (const name of cur.keys()) {
    if (!next.has(name)) lines.push(`remove RAG ${name}`);
  }
  if (current.llm.externalURL !== draft.llm.externalURL || current.llm.inCluster !== draft.llm.inCluster) {
    lines.push(`LLM endpoint ${current.llm.externalURL || "in-cluster"}→${draft.llm.externalURL || "in-cluster"}`);
  }
  const curChat = (current.llm.chatModels ?? []).join(",");
  const nextChat = (draft.llm.chatModels ?? []).join(",");
  if (curChat !== nextChat) {
    lines.push(`LLM chat models ${curChat || "—"}→${nextChat || "—"}`);
  }
  if ((current.llm.routerModel ?? "") !== (draft.llm.routerModel ?? "")) {
    lines.push(`LLM router model ${current.llm.routerModel || "—"}→${draft.llm.routerModel || "—"}`);
  }
  if (current.params.nResults !== draft.params.nResults) {
    lines.push(`params.nResults ${current.params.nResults}→${draft.params.nResults}`);
  }
  if (current.params.chunkCap !== draft.params.chunkCap) {
    lines.push(`params.chunkCap ${current.params.chunkCap}→${draft.params.chunkCap}`);
  }
  if (current.params.routerDefault !== draft.params.routerDefault) {
    lines.push(`params.routerDefault ${current.params.routerDefault}→${draft.params.routerDefault}`);
  }
  return lines;
}

const ROLLOUT_POLL_MS = 3000;

// useRollout polls the deployment API for rollout progress while active, so the
// panel shows the chatbot coming back after an apply.
export function useRollout(token: string, active: boolean): RolloutStatus | undefined {
  const [status, setStatus] = useState<RolloutStatus>();
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (!active || !token) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await fetchRollout(token);
        if (!cancelled) setStatus(s);
      } catch {
        /* transient during a rollout; keep polling */
      }
    };
    void tick();
    timer.current = window.setInterval(tick, ROLLOUT_POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer.current);
    };
  }, [token, active]);
  return status;
}

// useMeshState loads the deployed mesh view once a token is supplied.
export function useMeshState(token: string): {
  state?: MeshView;
  error?: string;
  loading: boolean;
  reload: () => void;
} {
  const [state, setState] = useState<MeshView>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const reload = useCallback(() => {
    if (!token) return;
    setLoading(true);
    setError(undefined);
    fetchMeshState(token)
      .then((s) => setState(s))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [token]);
  useEffect(() => {
    reload();
  }, [reload]);
  return { state, error, loading, reload };
}
