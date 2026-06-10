import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getTrace, getExperiment, TraceData, ExperimentConfig } from '../api/client'

export default function TraceViewer() {
  const { suite, ts, pointId } = useParams()

  const [trace, setTrace] = useState<TraceData | null>(null)
  const [experiment, setExperiment] = useState<ExperimentConfig | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [configExpanded, setConfigExpanded] = useState(false)

  useEffect(() => {
    if (!suite || !ts || !pointId) return
    Promise.all([
      getTrace(suite, ts, pointId),
      getExperiment(suite, ts, pointId).catch(() => null),
    ])
      .then(([t, e]) => { setTrace(t); setExperiment(e) })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [suite, ts, pointId])

  if (loading) return <div className="loading">Loading trace...</div>
  if (error) return <div className="error">{error}</div>
  if (!trace) return <div className="error">Trace not found</div>

  const maxDur = Math.max(...trace.spans.map(s => s.durationMs), 1)

  const machine = experiment?.harness?.machine as Record<string, unknown> | undefined
  const budget = machine?.budget as Record<string, unknown> | undefined
  const states = machine?.states as string[] | undefined
  const transitions = machine?.transitions as Record<string, unknown>[] | undefined

  const tools = experiment?.harness?.tools as Record<string, unknown> | undefined
  const toolList = (tools?.tools ?? []) as string[]

  const llmDecl = (experiment?.harness?.tool_declarations ?? []).find((d: Record<string, unknown>) => {
    const declTools = (d as Record<string, unknown>).tools as Record<string, unknown>[] | undefined
    return declTools?.some((t: Record<string, unknown>) => t.name === 'invoke_llm')
  })
  const llmTool = llmDecl
    ? ((llmDecl as Record<string, unknown>).tools as Record<string, unknown>[])?.find((t: Record<string, unknown>) => t.name === 'invoke_llm')
    : undefined
  const llmConfig = llmTool?.config as Record<string, unknown> | undefined

  return (
    <div>
      <Link to={`/sessions/${suite}/${ts}`} className="back-link">-- Session</Link>
      <h1>{pointId}</h1>

      {experiment && (
        <>
          <h2>Experiment Configuration</h2>
          <div className="config-panel">
            <div className="config-grid">
              <ConfigItem label="Model" value={experiment.model} />
              <ConfigItem label="Harness" value={experiment.harness.name} />
              <ConfigItem label="Binary" value={experiment.harness.binary} />
              <ConfigItem label="Sample" value={experiment.sample.name} />
              {experiment.agent_commit && <ConfigItem label="Agent Commit" value={experiment.agent_commit} />}
              {experiment.timeout && <ConfigItem label="Timeout" value={experiment.timeout} />}
              {experiment.ollama_url && <ConfigItem label="Ollama URL" value={experiment.ollama_url} />}
            </div>

            {budget && (
              <div className="config-section">
                <h3>Budget</h3>
                <div className="config-grid">
                  {Object.entries(budget).map(([k, v]) => (
                    <ConfigItem key={k} label={formatLabel(k)} value={String(v as string | number)} />
                  ))}
                </div>
              </div>
            )}

            {llmConfig && (
              <div className="config-section">
                <h3>LLM Configuration</h3>
                <div className="config-grid">
                  {'model' in llmConfig && <ConfigItem label="Model" value={String(llmConfig.model)} />}
                  {'provider' in llmConfig && <ConfigItem label="Provider" value={String(llmConfig.provider)} />}
                  {'provider_url' in llmConfig && <ConfigItem label="Provider URL" value={String(llmConfig.provider_url)} />}
                  {'max_time' in llmConfig && <ConfigItem label="Max Time" value={`${llmConfig.max_time}s`} />}
                </div>
                {'system_prompt' in llmConfig && (
                  <div className="config-prompt">
                    <h4>System Prompt</h4>
                    <pre className="config-prompt-text">{String(llmConfig.system_prompt).trim()}</pre>
                  </div>
                )}
              </div>
            )}

            {states && (
              <div className="config-section">
                <h3>State Machine</h3>
                <div className="config-states">
                  {states.map(s => (
                    <span key={s} className="config-state-badge">{s}</span>
                  ))}
                </div>
              </div>
            )}

            {toolList.length > 0 && (
              <div className="config-section">
                <h3>Registered Tools ({toolList.length})</h3>
                <div className="config-tools">
                  {toolList.map(t => (
                    <span key={t} className="config-tool-badge">{t}</span>
                  ))}
                </div>
              </div>
            )}

            {transitions && transitions.length > 0 && (
              <div className="config-section">
                <button
                  className="config-toggle"
                  onClick={() => setConfigExpanded(!configExpanded)}
                >
                  {configExpanded ? '- Hide' : '+ Show'} Transitions ({transitions.length})
                </button>
                {configExpanded && (
                  <div className="table-container" style={{ marginTop: 8 }}>
                    <table>
                      <thead>
                        <tr>
                          <th>State</th>
                          <th>Signal</th>
                          <th>Next</th>
                          <th>Action</th>
                        </tr>
                      </thead>
                      <tbody>
                        {transitions.map((t, i) => (
                          <tr key={i}>
                            <td className="mono">{String(t.state ?? '')}</td>
                            <td className="mono">{String(t.signal ?? '')}</td>
                            <td className="mono">{String(t.next ?? '')}</td>
                            <td className="mono">{String(t.action ?? '')}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}
          </div>
        </>
      )}

      <h2>Tool Call Timeline</h2>
      <div className="timeline">
        {trace.spans.map((span, i) => {
          const barWidth = Math.max((span.durationMs / maxDur) * 100, 2)
          const isLLM = span.name.startsWith('chat ')
          const isTool = !isLLM && span.toolName !== ''

          return (
            <div key={i} className="timeline-row">
              <div className="timeline-name">
                <span className={`timeline-badge ${isLLM ? 'badge-llm' : isTool ? 'badge-tool' : 'badge-other'}`}>
                  {isLLM ? 'LLM' : isTool ? 'TOOL' : 'SYS'}
                </span>
                <span className="mono">{span.toolName || span.name}</span>
              </div>
              <div className="timeline-bar-container">
                <div
                  className={`timeline-bar ${isLLM ? 'bar-llm' : isTool ? 'bar-tool' : 'bar-other'}`}
                  style={{ width: `${barWidth}%` }}
                />
              </div>
              <div className="timeline-meta">
                <span className="timeline-dur">{span.durationMs >= 1000 ? `${(span.durationMs/1000).toFixed(1)}s` : `${span.durationMs}ms`}</span>
                {span.tokensIn > 0 && (
                  <span className="timeline-tokens">{span.tokensIn} in / {span.tokensOut} out</span>
                )}
                {span.signal && <span className="timeline-signal">{span.signal}</span>}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function ConfigItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="config-item">
      <span className="config-label">{label}</span>
      <span className="config-value">{value}</span>
    </div>
  )
}

function formatLabel(key: string): string {
  return key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}
