const BASE = '/api/v1'

export async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  const body = await res.json()
  if (body.error) throw new Error(body.error)
  return body.data
}

export interface DocEntry {
  path: string
  name: string
  category: string
}

export interface DocDetail {
  path: string
  content: Record<string, unknown>
  raw: string
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

export interface ValidationFinding {
  code: string
  severity: string
  path: string
  message: string
}

export interface ValidationReport {
  status: string
  findings: ValidationFinding[]
  checked_paths: string[]
}

export interface SuggestionResponse {
  patch_id: string
  path: string
  status: string
  suggestions: string[]
  proposed_patch: string
  findings?: ValidationFinding[]
}

export interface PatchDecision {
  patch_id: string
  status: string
  decided_by: string
  applied: boolean
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const payload = await res.json()
  if (!res.ok && !payload.data) throw new Error(payload.error ?? `API error: ${res.status}`)
  return payload.data
}

async function postAction<T>(type: string, params: Record<string, unknown> = {}): Promise<T> {
  return postJSON<T>('/actions', { type, params })
}

export const listDocs = () => postAction<DocEntry[]>('doc_list')
export const getDoc = (path: string) => postAction<DocDetail>('doc_get', { path })
export const getConfig = (path: string) => fetchJSON<ConfigDetail>(`/configs/${path}`)
export const getSource = (path: string) => fetchJSON<SourceDetail>(`/source/${path}`)
export const validateDocs = (paths: string[], strict = false) => postAction<ValidationReport>('doc_validate', { paths, strict })
export const suggestDocChanges = (path: string, instruction: string, context = '') => {
  return postAction<SuggestionResponse>('doc_suggest_changes', { path, instruction, context })
}
export const approvePatch = (patchId: string, decidedBy: string, note = '') => {
  return postAction<PatchDecision>('doc_patch_approve', { patch_id: patchId, decided_by: decidedBy, note })
}
export const rejectPatch = (patchId: string, decidedBy: string, reason = '') => {
  return postAction<PatchDecision>('doc_patch_reject', { patch_id: patchId, decided_by: decidedBy, reason })
}
