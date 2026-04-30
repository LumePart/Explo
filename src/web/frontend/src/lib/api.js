export async function fetchConfig() {
  const res = await fetch('/api/config')
  return res.json()
}

export async function fetchConfigRaw() {
  const res = await fetch('/api/config/raw')
  return res.text()
}

export async function saveConfig(text) {
  const res = await fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: text,
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function resetConfig() {
  const res = await fetch('/api/config/reset', { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function saveSchedule(name, enabled, day, hour, minute) {
  const res = await fetch('/api/config/schedules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, enabled, day, hour, minute }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep1(user, playlists, discovery_mode) {
  const res = await fetch('/api/wizard/step1', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, playlists, discovery_mode }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep2(body) {
  const res = await fetch('/api/wizard/step2', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep3(body) {
  const res = await fetch('/api/wizard/step3', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function wizardStep4(body) {
  const res = await fetch('/api/wizard/step4', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function testLidarr(url, api_key) {
  const res = await fetch('/api/lidarr/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, api_key }),
  })
  return res.json()
}

export async function fetchLidarrProfiles(url, api_key) {
  const res = await fetch('/api/lidarr/profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, api_key }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchLidarrWebhookUrl() {
  const res = await fetch('/api/lidarr/webhook-url')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchBrowse(path) {
  const res = await fetch('/api/browse?path=' + encodeURIComponent(path || '/'))
  return res.json()
}

export async function startRun(playlist, download_mode, persist, exclude_local) {
  const form = new FormData()
  form.set('playlist', playlist)
  form.set('download_mode', download_mode)
  form.set('persist', persist ? 'true' : 'false')
  form.set('exclude_local', exclude_local ? 'true' : 'false')
  const res = await fetch('/api/run', { method: 'POST', body: form })
  if (res.status === 409) throw Object.assign(new Error('already running'), { conflict: true })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function stopRun() {
  const res = await fetch('/api/run/stop', { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchRunStatus() {
  const res = await fetch('/api/run/status')
  return res.json()
}

export async function fetchLogs() {
  const res = await fetch('/api/logs')
  return res.text()
}
