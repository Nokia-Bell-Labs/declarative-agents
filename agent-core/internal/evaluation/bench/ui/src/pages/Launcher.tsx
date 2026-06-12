import { useState, useEffect } from 'react'
import { listConfigs, postAction, type ConfigCategory } from '../api/client'

export default function Launcher() {
  const [configs, setConfigs] = useState<ConfigCategory[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [suitePath, setSuitePath] = useState('')
  const [outputDir, setOutputDir] = useState('eval-results')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<{ type: 'success' | 'error'; message: string } | null>(null)

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
      await postAction({
        type: 'launch_eval',
        config: {
          suite: suitePath.trim(),
          output_dir: outputDir.trim() || undefined,
        },
      })
      setResult({ type: 'success', message: `Experiment launched: ${suitePath}` })
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
    </div>
  )
}
