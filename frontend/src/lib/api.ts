let _port = 0

export function setBackendPort(port: number): void {
  _port = port
}

export function getBackendPort(): number {
  return _port
}

export function apiUrl(path: string): string {
  return `http://127.0.0.1:${_port}${path}`
}

export async function api<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(apiUrl(path), init)
  if (!res.ok) {
    let errMsg = res.statusText
    try {
      const body = await res.json()
      errMsg = body.error || errMsg
    } catch {}
    throw new Error(errMsg)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}
