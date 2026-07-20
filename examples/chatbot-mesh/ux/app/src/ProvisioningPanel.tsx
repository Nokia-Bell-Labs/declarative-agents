import { useEffect, useMemo, useState } from "react";
import {
  type MeshView,
  type RagView,
  applyMesh,
  diffMesh,
  useMeshState,
  useRollout,
} from "./provisioningApi";

// The provisioning panel (srd002 R5 / srd003 R4): the UX surface for viewing and
// changing the mesh. It drives the deployment API only — it reads the deployed
// mesh view and submits values patches that trigger a rollout, and never submits a
// host or URL to a running agent. The apply token is entered here and held in
// memory for the session, separate from the chat path.
export default function ProvisioningPanel() {
  const [token, setToken] = useState("");
  const [applied, setApplied] = useState(false);
  const { state, error, loading, reload } = useMeshState(token);
  const [draft, setDraft] = useState<MeshView>();
  const [applyError, setApplyError] = useState<string>();
  const [applying, setApplying] = useState(false);
  const rollout = useRollout(token, applied);

  // Seed the editable draft from the deployed state whenever it (re)loads.
  useEffect(() => {
    if (state) setDraft(structuredClone(state));
  }, [state]);

  const diff = useMemo(
    () => (state && draft ? diffMesh(state, draft) : []),
    [state, draft],
  );

  if (!token) {
    return (
      <div className="provisioning">
        <h2>Provisioning</h2>
        <p className="prov-hint">
          The provisioning panel drives the deployment API to view and change the
          mesh. Enter an apply token (separate from chat) to load the deployed
          state. A read-only token loads the view but cannot apply.
        </p>
        <TokenGate onSubmit={setToken} />
      </div>
    );
  }

  return (
    <div className="provisioning">
      <div className="prov-head">
        <h2>Provisioning</h2>
        <button className="prov-btn" onClick={reload} disabled={loading}>
          {loading ? "loading…" : "reload"}
        </button>
      </div>

      {error && <div className="prov-error">read failed: {error}</div>}
      {!draft && !error && <p className="prov-hint">Loading deployed mesh…</p>}

      {draft && (
        <>
          <section className="prov-section">
            <h3>RAG units</h3>
            <div className="prov-rags">
              {draft.rags.map((rag, i) => (
                <RagCard
                  key={i}
                  rag={rag}
                  onChange={(next) => setDraft(withRag(draft, i, next))}
                  onRemove={() => setDraft(removeRag(draft, i))}
                />
              ))}
            </div>
            <button className="prov-btn" onClick={() => setDraft(addRag(draft))}>
              + add RAG
            </button>
          </section>

          <section className="prov-section">
            <h3>LLM endpoint</h3>
            <label className="prov-field">
              <span>external URL</span>
              <input
                value={draft.llm.externalURL}
                onChange={(e) =>
                  setDraft({ ...draft, llm: { ...draft.llm, externalURL: e.target.value } })
                }
              />
            </label>
            <label className="prov-field">
              <span>chat model</span>
              <input
                value={draft.llm.chatModel}
                onChange={(e) =>
                  setDraft({ ...draft, llm: { ...draft.llm, chatModel: e.target.value } })
                }
              />
            </label>
          </section>

          {draft.llm.inCluster && (
            <section className="prov-section prov-llm-tier">
              <h3>In-cluster LLM tier</h3>
              <p className="prov-note">
                {draft.llm.topology === "per-model" ? "one Ollama per model" : "single Ollama"}, preloaded once — the
                models named here flow to both the preload Job and the agents' config.
              </p>
              <label className="prov-field">
                <span>embedding model</span>
                <input
                  value={draft.llm.embedModel}
                  onChange={(e) =>
                    setDraft({ ...draft, llm: { ...draft.llm, embedModel: e.target.value } })
                  }
                />
              </label>
              <label className="prov-field">
                <span>router model</span>
                <input
                  value={draft.llm.routerModel ?? ""}
                  onChange={(e) =>
                    setDraft({ ...draft, llm: { ...draft.llm, routerModel: e.target.value } })
                  }
                />
              </label>
              <label className="prov-field">
                <span>chat models</span>
                <input
                  value={(draft.llm.chatModels ?? []).join(", ")}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      llm: {
                        ...draft.llm,
                        chatModels: e.target.value
                          .split(",")
                          .map((s) => s.trim())
                          .filter(Boolean),
                      },
                    })
                  }
                />
              </label>
            </section>
          )}

          <section className="prov-section">
            <h3>Parameters</h3>
            <label className="prov-field">
              <span>n_results (per-RAG top-k)</span>
              <input
                type="number"
                value={draft.params.nResults}
                onChange={(e) =>
                  setDraft({ ...draft, params: { ...draft.params, nResults: Number(e.target.value) } })
                }
              />
            </label>
            <label className="prov-field">
              <span>chunk cap (0 = uncapped)</span>
              <input
                type="number"
                value={draft.params.chunkCap}
                onChange={(e) =>
                  setDraft({ ...draft, params: { ...draft.params, chunkCap: Number(e.target.value) } })
                }
              />
            </label>
            <label className="prov-field">
              <span>router default word</span>
              <input
                value={draft.params.routerDefault}
                onChange={(e) =>
                  setDraft({ ...draft, params: { ...draft.params, routerDefault: e.target.value } })
                }
              />
            </label>
          </section>

          <section className="prov-section">
            <h3>Diff preview</h3>
            {diff.length === 0 ? (
              <p className="prov-hint">No changes from the deployed mesh.</p>
            ) : (
              <ul className="prov-diff">
                {diff.map((line, i) => (
                  <li key={i}>{line}</li>
                ))}
              </ul>
            )}
            <button
              className="prov-btn prov-apply"
              disabled={applying || diff.length === 0}
              onClick={async () => {
                if (!draft) return;
                setApplying(true);
                setApplyError(undefined);
                try {
                  await applyMesh(token, draft);
                  setApplied(true);
                } catch (e) {
                  setApplyError((e as Error).message);
                } finally {
                  setApplying(false);
                }
              }}
            >
              {applying ? "applying…" : "apply & roll out"}
            </button>
            {applyError && <div className="prov-error">apply failed: {applyError}</div>}
          </section>

          {applied && (
            <section className="prov-section">
              <h3>Rollout</h3>
              {rollout ? (
                <div className="prov-rollout">
                  <span className={`dot ${rollout.phase === "complete" ? "dot-ok" : "dot-idle"}`} />
                  <span>
                    {rollout.phase} — {rollout.ready}/{rollout.desired} ready (rev {rollout.revision})
                  </span>
                </div>
              ) : (
                <p className="prov-hint">Polling rollout…</p>
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
}

function TokenGate({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <form
      className="prov-token"
      onSubmit={(e) => {
        e.preventDefault();
        if (value.trim()) onSubmit(value.trim());
      }}
    >
      <input
        type="password"
        placeholder="deployment API token"
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      <button className="prov-btn" type="submit">
        load mesh
      </button>
    </form>
  );
}

function RagCard({
  rag,
  onChange,
  onRemove,
}: {
  rag: RagView;
  onChange: (next: RagView) => void;
  onRemove: () => void;
}) {
  return (
    <div className="prov-rag">
      <div className="prov-rag-head">
        <input
          className="prov-rag-name"
          value={rag.name}
          onChange={(e) => onChange({ ...rag, name: e.target.value })}
        />
        {rag.status && <span className="prov-rag-status">{rag.status}</span>}
        <button className="prov-rag-remove" onClick={onRemove} title="remove RAG">
          ×
        </button>
      </div>
      <label className="prov-field">
        <span>collection</span>
        <input value={rag.collection} onChange={(e) => onChange({ ...rag, collection: e.target.value })} />
      </label>
      <label className="prov-field">
        <span>embedding model</span>
        <input
          value={rag.embeddingModel}
          onChange={(e) => onChange({ ...rag, embeddingModel: e.target.value })}
        />
      </label>
      <label className="prov-field">
        <span>replicas</span>
        <input
          type="number"
          value={rag.replicas}
          onChange={(e) => onChange({ ...rag, replicas: Number(e.target.value) })}
        />
      </label>
    </div>
  );
}

function withRag(mesh: MeshView, i: number, next: RagView): MeshView {
  const rags = mesh.rags.slice();
  rags[i] = next;
  return { ...mesh, rags };
}

function removeRag(mesh: MeshView, i: number): MeshView {
  return { ...mesh, rags: mesh.rags.filter((_, idx) => idx !== i) };
}

function addRag(mesh: MeshView): MeshView {
  const n = mesh.rags.length;
  return {
    ...mesh,
    rags: [
      ...mesh.rags,
      { name: `rag${n}`, collection: `corpus${n}`, embeddingModel: mesh.llm.embedModel || "qwen3-embedding:8b", replicas: 1 },
    ],
  };
}
