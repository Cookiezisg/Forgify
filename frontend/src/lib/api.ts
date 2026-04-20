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

const DEFAULT_TIMEOUT = 30_000 // 30 seconds

export async function api<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), DEFAULT_TIMEOUT)

  try {
    const res = await fetch(apiUrl(path), {
      ...init,
      signal: controller.signal,
    })
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
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') {
      throw new Error('请求超时，请检查网络连接')
    }
    throw e
  } finally {
    clearTimeout(timeout)
  }
}
