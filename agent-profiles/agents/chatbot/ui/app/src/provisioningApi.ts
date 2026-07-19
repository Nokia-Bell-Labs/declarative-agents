import { useCallback, useEffect, useRef, useState } from "react";

// The deployment API the provisioning panel drives (srd015 R4). It is same-origin
// at /provisioning, routed by the chatbot ingress to the provisioner Service, so
// the panel's POST apply avoids the GET-only monitor_proxy and no call crosses
// origin. The panel never calls an agent endpoint; provisioning changes deployment
// values and triggers rollouts only (R4.2). The base is values-driven via ux.yaml;
// it defaults to the same-origin path.
export const PROVISIONING_BASE =
  (globalThis as { PROVISIONING_API?: string }).PROVISIONING_API ?? "/provisioning/api";

// MeshView mirrors the provisioner's values-plane view: RAG topology, the LLM
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

export interface RolloutStatus {
  phase: "progressing" | "complete" | "unknown";
  ready: number;
  desired: number;
  revision: number;
  message?: string;
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
  return (await res.json()) as MeshView;
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
