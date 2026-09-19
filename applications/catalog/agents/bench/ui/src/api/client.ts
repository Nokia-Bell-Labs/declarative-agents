import type { KitClient } from '@declarative-agents/ui-kit'

// Every bench read goes through the kit client the shell provides (srd004
// R6.1), so the same pages run same-origin or against another base URL. The
// bench answers {data} on success and {error} on a domain failure.
const BASE = '/api/v1'

export async function fetchJSON<T>(client: KitClient, path: string): Promise<T> {
  const body = await client.getJSON<{ data: T; error?: string }>(`${BASE}${path}`)
  if (body.error) throw new Error(body.error)
  return body.data
}

export interface Session {
  id: string
  name: string
  timestamp: string
  pointCount: number
  passCount: number
  failCount: number
  timeoutCount: number
}

export interface ModelStat {
  model: string
  runs: number
  successes: number
  successRate: number
  cleanRate: number
  recoveryRate: number
  stuckRate: number
  meanIter: number
  meanTokensIn: number
  meanTokensOut: number
  meanDurationS: number
}

export interface SampleStat {
  sample: string
  model: string
  runs: number
  successRate: number
  meanIter: number
  meanTokens: number
  meanDurationS: number
}

export interface SessionDetail {
  id: string
  modelStats: ModelStat[]
  sampleStats: SampleStat[]
  totalPoints: number
  totalPassed: number
  totalFailed: number
  totalTimedOut: number
}

export interface Point {
  pointId: string
  sample: string
  model: string
  testsPassed: boolean
  timedOut: boolean
  exitCode: number
  durationS: number
  iterations: number
  tokensIn: number
  tokensOut: number
  convergence: string
}

export interface TraceSpan {
  name: string
  startTime: string
  endTime: string
  durationMs: number
  toolName: string
  signal: string
  tokensIn: number
  tokensOut: number
}

export interface TraceData {
  pointId: string
  spans: TraceSpan[]
  snapshots: unknown[]
}

export interface ExperimentConfig {
  agent_commit?: string
  harness: {
    name: string
    binary: string
    machine?: Record<string, unknown>
    tools?: Record<string, unknown>
    tool_declarations?: Record<string, unknown>[]
  }
  model: string
  ollama_url?: string
  timeout?: string
  sample: { name: string }
}

export interface ConfigFile {
  path: string
  name: string
}

export interface ConfigCategory {
  category: string
  files: ConfigFile[]
}

export interface ConfigDetail {
  path: string
  content: Record<string, unknown>
  raw: string
  graph?: {
    states: string[]
    terminalStates: string[]
    transitions: { from: string; signal: string; to: string; action?: string }[]
  }
}

export interface SourceDetail {
  path: string
  content: string
  language: string
  mimeType: string
  size: number
}

export const listConfigs = (client: KitClient) => fetchJSON<ConfigCategory[]>(client, '/configs')
export const getConfig = (client: KitClient, path: string) => fetchJSON<ConfigDetail>(client, `/configs/${path}`)
export const getSource = (client: KitClient, path: string) => fetchJSON<SourceDetail>(client, `/source/${path}`)

export interface ExperimentLaunch {
  suite: string
  output_dir: string
}

export interface LaunchedExperiment {
  service: string
  pid: number
  started_at: string
}

// A critic run the bench started (srd006 R3.5): running, or exited with the
// exit code the host recorded when it reaped the child.
export interface ExperimentRun {
  service: string
  pid: number
  status: 'running' | 'exited'
  exit_code?: number
  started_at: string
  finished_at?: string
}

// The launch is a POST through the kit client's request init.
export async function launchExperiment(client: KitClient, launch: ExperimentLaunch): Promise<LaunchedExperiment> {
  const res = await client.request(`${BASE}/experiments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(launch),
  })
  const body = await res.json().catch(() => ({}))
  if (res.status === 429) {
    throw new Error(`Too many experiments running (limit ${body.max_running ?? 'reached'}); wait for one to finish.`)
  }
  if (!res.ok) throw new Error(body.error || `Launch failed: ${res.status}`)
  return body as LaunchedExperiment
}

export const listExperiments = (client: KitClient) => fetchJSON<ExperimentRun[]>(client, '/experiments/runs')

export const listSessions = (client: KitClient) => fetchJSON<Session[]>(client, '/sessions')
export const getSession = (client: KitClient, suite: string, ts: string) =>
  fetchJSON<SessionDetail>(client, `/sessions/${suite}/${ts}`)
export const listPoints = (client: KitClient, suite: string, ts: string) =>
  fetchJSON<Point[]>(client, `/sessions/${suite}/${ts}/points`)
export const getTrace = (client: KitClient, suite: string, ts: string, pointId: string) =>
  fetchJSON<TraceData>(client, `/sessions/${suite}/${ts}/points/${pointId}`)
export const getExperiment = (client: KitClient, suite: string, ts: string, pointId: string) =>
  fetchJSON<ExperimentConfig>(client, `/sessions/${suite}/${ts}/points/${pointId}/experiment`)
