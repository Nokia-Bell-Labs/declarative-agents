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

export const listDocs = () => fetchJSON<DocEntry[]>('/docs')
export const getDoc = (path: string) => fetchJSON<DocDetail>(`/docs/${path}`)
export const getConfig = (path: string) => fetchJSON<ConfigDetail>(`/configs/${path}`)
export const getSource = (path: string) => fetchJSON<SourceDetail>(`/source/${path}`)
