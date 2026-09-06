import { start, finish } from './mb.svelte.js'

async function tracked(path) {
  const id = start(path)
  try {
    return await request(path)
  } finally {
    finish(id)
  }
}

async function request(path, options) {
  const res = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    throw new Error((await res.text()).trim() || `Error ${res.status}`)
  }
  return res.json()
}

const shortWait = () => ({ signal: AbortSignal.timeout(10000) })

export const getStatus = () => request('/status', shortWait())
export const getSettings = () => request('/settings')
export const previewPattern = (body) =>
  request('/settings/preview', { method: 'POST', body: JSON.stringify(body) })
export const saveSettings = (changes) =>
  request('/settings', { method: 'POST', body: JSON.stringify(changes) })
export const clearCache = () => request('/settings/cache/clear', { method: 'POST' })
export const getAlbums = (status) =>
  request(`/albums?status=${encodeURIComponent(status ?? '')}`, shortWait())
export const getAlbum = (id) => request(`/albums/${id}`)
export const scan = () => request('/scan', { method: 'POST' })

export function getCandidates(id, filters = {}, force = false) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value) params.set(key, value)
  }
  if (force) params.set('force', '1')
  const query = params.toString()
  return tracked(`/albums/${id}/candidates${query ? `?${query}` : ''}`)
}

export const getFiled = (limit = 50) => request(`/filed?limit=${limit}`)

export const getRelease = (mbid) => tracked(`/releases/${mbid}`)
export const getPlan = (id, releaseId, mode) =>
  request(`/albums/${id}/plan?release_id=${encodeURIComponent(releaseId)}&mode=${mode}`)

export const apply = (id, releaseId, mode) =>
  request(`/albums/${id}/apply`, {
    method: 'POST',
    body: JSON.stringify({ release_id: releaseId, mode }),
  })
