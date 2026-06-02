let csrfToken = null

async function ensureCSRF() {
  if (csrfToken) return csrfToken

  const res = await fetch('/api/ui/csrf')
  const data = await res.json()
  csrfToken = data.csrf_token
  return csrfToken
}

async function apiFetch(url, options = {}) {
  const method = (options.method || 'GET').toUpperCase()

  const headers = new Headers(options.headers || {})

  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const token = await ensureCSRF()
    headers.set('X-CSRF-Token', token)
  }

  return fetch(url, {
    credentials: 'include',
    ...options,
    headers,
  })
}

export async function checkAuth() {
  const res = await fetch('/api/ui/auth/status', { credentials: 'include' })
  return res.ok
}

export async function login(username, password) {
  const form = new URLSearchParams()
  form.append('username', username)
  form.append('password', password)

  const res = await apiFetch('/api/ui/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: form.toString(),
  })

  if (!res.ok) {
    throw new Error(await res.text())
  }
}

export async function fetchConfig() {
  const res = await apiFetch('/api/ui/config')
  return res.json()
}

export async function fetchConfigRaw() {
  const res = await apiFetch('/api/ui/config/raw')
  return res.text()
}

export async function saveConfig(text) {
  const res = await apiFetch('/api/ui/config', {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: text,
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function resetConfig() {
  const res = await apiFetch('/api/ui/config/reset', { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function saveSchedule(name, enabled, day, hour, minute) {
  const res = await apiFetch('/api/ui/config/schedules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, enabled, day, hour, minute }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep1(user, playlists, discovery_mode) {
  const res = await apiFetch('/api/ui/wizard/step1', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, playlists, discovery_mode }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep2(body) {
  const res = await apiFetch('/api/ui/wizard/step2', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep3(body) {
  const res = await apiFetch('/api/ui/wizard/step3', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchBrowse(path) {
  const res = await apiFetch('/api/ui/browse?path=' + encodeURIComponent(path || '/'))
  return res.json()
}

export async function startRun(playlist, download_mode, persist, exclude_local) {
  const form = new FormData()
  form.set('playlist', playlist)
  form.set('download_mode', download_mode)
  form.set('persist', persist ? 'true' : 'false')
  form.set('exclude_local', exclude_local ? 'true' : 'false')
  const res = await apiFetch('/api/ui/run', { method: 'POST', body: form })
  if (res.status === 409) throw Object.assign(new Error('already running'), { conflict: true })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function stopRun() {
  const res = await apiFetch('/api/ui/run/stop', { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchRunStatus() {
  const res = await apiFetch('/api/ui/run/status')
  return res.json()
}

export async function fetchLogs() {
  const res = await apiFetch('/api/ui/logs')
  return res.text()
}

export async function prefetchPlaylists(user, playlists, options = {}) {
  await apiFetch('/api/ui/playlists/prefetch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, playlists, ...options }),
  })
}

export async function logout() {
  await apiFetch('/api/ui/logout', { method: 'POST' })
}

export async function fetchSetupStatus() {
  try {
    const res = await fetch('/api/ui/setup-status')
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}

export async function fetchCustomPlaylists() {
  const res = await apiFetch('/api/ui/custom-playlists')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function importCustomPlaylist(url, source, refresh_days) {
  const res = await apiFetch('/api/ui/custom-playlists', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, source, refresh_days }),
  })
  if (res.status === 409) throw Object.assign(new Error('already imported'), { duplicate: true })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function deleteCustomPlaylist(id, { deleteTracks = false } = {}) {
  const qs = deleteTracks ? '?delete_tracks=true' : ''
  const res = await apiFetch(`/api/ui/custom-playlists/${encodeURIComponent(id)}${qs}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}

export async function refreshCustomPlaylist(id) {
  const res = await apiFetch(`/api/ui/custom-playlists/${encodeURIComponent(id)}/refresh`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchBackgroundArt() {
  try {
    const res = await fetch('/api/ui/background-art')
    if (!res.ok) return null
    const { url } = await res.json()
    return url || null
  } catch {
    return null
  }
}
