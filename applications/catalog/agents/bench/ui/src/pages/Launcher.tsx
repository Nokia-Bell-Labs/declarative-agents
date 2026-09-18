import { useState, useEffect, useCallback } from 'react'
import {
  listConfigs, launchExperiment, listExperiments,
  type ConfigCategory, type ExperimentRun,
} from '../api/client'

const RUN_POLL_MS = 5000

export default function Launcher() {
  const [configs, setConfigs] = useState<ConfigCategory[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [suitePath, setSuitePath] = useState('')
  const [outputDir, setOutputDir] = useState('eval-results')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [runs, setRuns] = useState<ExperimentRun[]>([])
  const [runsError, setRunsError] = useState<string | null>(null)

  const refreshRuns = useCallback(() => {
    listExperiments()
      .then(data => { setRuns(data ?? []); setRunsError(null) })
      .catch(() => setRunsError('Failed to load experiment runs'))
  }, [])

  useEffect(() => { refreshRuns() }, [refreshRuns])

  // Poll only while a run is live; an idle launcher makes no requests.
  const anyRunning = runs.some(run => run.status === 'running')
  useEffect(() => {
    if (!anyRunning) return
    const timer = setInterval(refreshRuns, RUN_POLL_MS)
    return () => clearInterval(timer)
  }, [anyRunning, refreshRuns])

  useEffect(() => {
    listConfigs()
      .then(setConfigs)
      .catch(() => setError('Failed to load configs'))
      .finally(() => setLoading(false))
  }, [])

  const suiteFiles = configs
    .flatMap(cat => cat.files)
    .filter(f => f.path.includes('suite') || f.name.includes('suite'))

  const handleLaunch = async () => {
    if (!suitePath.trim()) return

    setSubmitting(true)
    setResult(null)
    try {
      const launched = await launchExperiment({
        suite: suitePath.trim(),
        output_dir: outputDir.trim() || 'eval-results',
      })
      setResult({ type: 'success', message: `Experiment launched: ${suitePath} (run ${launched.service})` })
      refreshRuns()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Launch failed'
      setResult({ type: 'error', message: msg })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <h1>Launch Experiment</h1>
      <p className="launcher-subtitle">
        Configure and launch an evaluation suite through the bench agent.
      </p>

      {error && <div className="error">{error}</div>}

      <div className="launcher-form">
        <div className="launcher-field">
          <label className="launcher-label" htmlFor="suite-path">Suite Path</label>
          <div className="launcher-input-group">
            <input
              id="suite-path"
              type="text"
              className="launcher-input"
              placeholder="e.g. eval-suites/coding/suite.yaml"
              value={suitePath}
              onChange={e => setSuitePath(e.target.value)}
              disabled={submitting}
            />
          </div>
          {!loading && suiteFiles.length > 0 && (
            <div className="launcher-suggestions">
              <span className="launcher-suggestions-label">Available suites:</span>
              {suiteFiles.map(f => (
                <button
                  key={f.path}
                  className="launcher-suggestion"
                  onClick={() => setSuitePath(`configs/${f.path}`)}
                  disabled={submitting}
                >
                  {f.path}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="launcher-field">
          <label className="launcher-label" htmlFor="output-dir">Output Directory</label>
          <input
            id="output-dir"
            type="text"
            className="launcher-input"
            placeholder="eval-results"
            value={outputDir}
            onChange={e => setOutputDir(e.target.value)}
            disabled={submitting}
          />
        </div>

        <div className="launcher-actions">
          <button
            className="launcher-button"
            onClick={handleLaunch}
            disabled={!suitePath.trim() || submitting}
          >
            {submitting ? 'Launching...' : 'Launch Experiment'}
          </button>
        </div>

        {result && (
          <div className={`launcher-result launcher-result-${result.type}`}>
            {result.message}
          </div>
        )}
      </div>

      <div className="launcher-runs">
        <h2>Experiment Runs</h2>
        {runsError && <div className="error">{runsError}</div>}
        {!runsError && runs.length === 0 && (
          <div className="empty">No experiments started since the bench launched.</div>
        )}
        {runs.length > 0 && (
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Run</th>
                  <th>Status</th>
                  <th>Exit code</th>
                  <th>Started</th>
                  <th>Finished</th>
                </tr>
              </thead>
              <tbody>
                {runs.map(run => (
                  <tr key={run.service}>
                    <td>{run.service}</td>
                    <td className={runStatusClass(run)}>{run.status}</td>
                    <td>{run.exit_code ?? ''}</td>
                    <td>{run.started_at}</td>
                    <td>{run.finished_at ?? ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function runStatusClass(run: ExperimentRun): string {
  if (run.status === 'running') return ''
  return run.exit_code === 0 ? 'pass' : 'fail'
}
