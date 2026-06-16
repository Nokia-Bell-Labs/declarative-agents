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

export interface UXConfig {
  title: string
  routes: UXRoute[]
  sidebar: UXSidebar
  actions: Record<string, UXAction>
  presentation: UXPresentation
}

export interface UXRoute {
  id: string
  path: string
  label: string
  action: string
  resource: string
}

export interface UXSidebar {
  title: string
  groups: Record<string, UXGroup>
}

export interface UXGroup {
  label: string
  order: number
}

export interface UXAction {
  ui_action: string
  request_machine_action?: string
  route: string
}

export interface UXPresentation {
  raw_yaml_toggle: boolean
  state_diagram: boolean
  config_viewer: boolean
  source_viewer: boolean
}

let uxConfig: Promise<UXConfig> | null = null

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

function documentPath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export function getUXConfig(): Promise<UXConfig> {
  uxConfig ??= fetchJSON<UXConfig>('/ux')
  return uxConfig
}

async function uiAction(name: string): Promise<string> {
  const cfg = await getUXConfig()
  const action = cfg.actions[name]
  if (!action) throw new Error(`missing UX action: ${name}`)
  return action.ui_action
}

async function routePath(id: string): Promise<string> {
  const cfg = await getUXConfig()
  const route = cfg.routes.find(item => item.id === id)
  if (!route) throw new Error(`missing UX route: ${id}`)
  return route.path.replace(/\/\*$/, '')
}

async function fetchDoc(path: string): Promise<DocDetail> {
  const detailRoute = await routePath('docs_detail')
  const res = await fetch(`${BASE}${detailRoute}/${documentPath(path)}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  const body = await res.json()
  if (body.error) throw new Error(body.error)
  return {
    path: body.path ?? path,
    content: body.data ?? body.content ?? {},
    raw: body.raw ?? '',
  }
}

export const listDocs = async () => fetchJSON<DocEntry[]>(await routePath('docs_index'))
export const getDoc = (path: string) => fetchDoc(path)
export const getConfig = (path: string) => fetchJSON<ConfigDetail>(`/configs/${path}`)
export const getSource = (path: string) => fetchJSON<SourceDetail>(`/source/${path}`)
export const validateDocs = async (paths: string[], strict = false) => {
  return postAction<ValidationReport>(await uiAction('validate_document'), { paths, strict })
}
export const suggestDocChanges = async (path: string, instruction: string, context = '') => {
  return postAction<SuggestionResponse>(await uiAction('suggest_changes'), { path, instruction, context })
}
export const approvePatch = async (patchId: string, decidedBy: string, note = '') => {
  return postAction<PatchDecision>(await uiAction('approve_patch'), { patch_id: patchId, decided_by: decidedBy, note })
}
export const rejectPatch = async (patchId: string, decidedBy: string, reason = '') => {
  return postAction<PatchDecision>(await uiAction('reject_patch'), { patch_id: patchId, decided_by: decidedBy, reason })
}
