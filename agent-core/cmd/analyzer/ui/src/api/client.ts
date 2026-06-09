const BASE = '/api/v1'

export async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  const body = await res.json()
  if (body.error) {
    throw new Error(body.error)
  }
  return body.data
}
