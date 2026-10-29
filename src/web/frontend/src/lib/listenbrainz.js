// Session-level cache — avoids repeat fetches on open/close within the same page load.
const memCache = new Map()

const cacheKey = (playlistType, user) => `${user || ''}:${playlistType}`

export function clearPlaylistCache(playlistType, user) {
  if (playlistType) memCache.delete(cacheKey(playlistType, user))
  else memCache.clear()
}

export async function fetchPlaylistTracks(playlistType, options = {}) {
  const key = cacheKey(playlistType, options.user)
  if (!options.force && memCache.has(key)) return memCache.get(key)

  const res = await fetch(`/api/ui/playlists?type=${encodeURIComponent(playlistType)}${options.user ? `&user=${encodeURIComponent(options.user)}` : ''}`, {
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
