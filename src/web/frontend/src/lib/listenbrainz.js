// Session-level cache — avoids repeat fetches on open/close within the same page load.
const memCache = new Map()

export async function fetchPlaylistTracks(playlistType, options = {}) {
  const key = playlistType
  if (!options.force && memCache.has(key)) return memCache.get(key)

  const res = await fetch(`/api/ui/playlists?type=${encodeURIComponent(playlistType)}`, {
    credentials: 'include',
  })
  if (res.status === 404) {
    const result = { tracks: [], generatedAt: null }
    memCache.set(key, result)
    return result
  }
  if (!res.ok) throw new Error(`Server returned ${res.status}`)
  const data = await res.json()
  const result = { tracks: data.tracks ?? [], generatedAt: data.generatedAt ?? null }
  if (result.tracks.length > 0) {
    memCache.set(key, result)
  }
  return result
}
