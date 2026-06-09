const BASE = '/api/v1'

export async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  const body = await res.json()
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

export const listSessions = () => fetchJSON<Session[]>('/sessions')
export const getSession = (suite: string, ts: string) => fetchJSON<SessionDetail>(`/sessions/${suite}/${ts}`)
export const listPoints = (suite: string, ts: string) => fetchJSON<Point[]>(`/sessions/${suite}/${ts}/points`)
export const getTrace = (suite: string, ts: string, pointId: string) => fetchJSON<TraceData>(`/sessions/${suite}/${ts}/points/${pointId}`)
