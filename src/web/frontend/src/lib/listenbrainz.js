// Session-level cache — avoids repeat fetches on open/close within the same page load.
const memCache = new Map()

export async function fetchPlaylistTracks(playlistType) {
  const key = playlistType
  if (memCache.has(key)) return memCache.get(key)

  const res = await fetch(`/api/playlists?type=${encodeURIComponent(playlistType)}`)
  if (res.status === 404) {
    const result = { tracks: [], generatedAt: null }
    memCache.set(key, result)
    return result
  }
  if (!res.ok) throw new Error(`Server returned ${res.status}`)
  const data = await res.json()
  const result = { tracks: data.tracks ?? [], generatedAt: data.generatedAt ?? null }
  memCache.set(key, result)
  return result
}
