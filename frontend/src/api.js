// Thin fetch wrapper around the backend OTT integration service. Keeps the
// API base URL configurable per environment and centralizes error handling.

const BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

async function request(path, { method = 'GET', body, signal } = {}) {
  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    signal,
  })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const err = new Error(data?.message || data?.error || `HTTP ${res.status}`)
    err.status = res.status
    err.code = data?.error
    throw err
  }
  return data
}

export const api = {
  subscribe: (payload, signal) =>
    request('/api/subscribe', { method: 'POST', body: payload, signal }),
  activate: (activationCode, signal) =>
    request('/api/activate', { method: 'POST', body: { activationCode }, signal }),
  status: (activationCode, { refresh = false } = {}, signal) =>
    request(
      `/api/subscription-status?activationCode=${encodeURIComponent(activationCode)}${refresh ? '&refresh=true' : ''}`,
      { signal },
    ),
  providers: (signal) => request('/api/providers', { signal }),
}
