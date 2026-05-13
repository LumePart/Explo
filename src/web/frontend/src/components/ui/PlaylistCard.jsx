import { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { Toggle } from './Toggle'
import { Button } from './common'
import { fetchPlaylistTracks } from '../../lib/listenbrainz'
import { prefetchPlaylists } from '../../lib/api'

// ── TrackRow ──────────────────────────────────────────────────────────────────

function TrackRow({ track }) {
  const [imgFailed, setImgFailed] = useState(false)

  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 12,
      padding: '0 2px', minHeight: 52,
      borderBottom: '1px solid rgba(255,255,255,0.04)',
    }}>
      <span style={{
        width: 24, fontSize: 11, color: '#3a3a3a', textAlign: 'right',
        flexShrink: 0, fontVariantNumeric: 'tabular-nums',
      }}>
        {track.rank}
      </span>

      <div style={{
        width: 42, height: 42, borderRadius: 3, flexShrink: 0,
        background: '#1e1e1e', overflow: 'hidden',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        {track.coverUrl && !imgFailed ? (
          <img
            src={track.coverUrl}
            alt=""
            loading="lazy"
            style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
            onError={() => setImgFailed(true)}
          />
        ) : (
          <span style={{ fontSize: 14, color: '#2e2e2e' }}>♪</span>
        )}
      </div>

      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{
          fontSize: 13, color: 'white', fontWeight: 500,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {track.title}
        </div>
        <div style={{
          fontSize: 11, color: '#4e4e4e', marginTop: 2,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {track.artist}{track.release ? ` — ${track.release}` : ''}
        </div>
      </div>

      {track.inLibrary != null && (
        <span title={track.inLibrary ? 'Added to library' : 'Not added'} style={{
          flexShrink: 0, fontSize: 10, fontWeight: 600,
          color: track.inLibrary ? '#34d399' : '#3a3a3a',
          letterSpacing: '0.04em',
        }}>
          {track.inLibrary ? '✓' : '✕'}
        </span>
      )}

    </div>
  )
}

// ── TracklistDropdown ─────────────────────────────────────────────────────────

function nextUpdateLabel(playlistType) {
  const now = new Date()
  if (playlistType === 'on-repeat') {
    return 'Updates as you listen'
  }
  if (playlistType === 'daily-jams') {
    const tomorrow = new Date(now)
    tomorrow.setDate(tomorrow.getDate() + 1)
    return `Next update tomorrow (${tomorrow.toLocaleDateString([], { weekday: 'long' })})`
  }
  // Weekly playlists: LB generates on Mondays
  const daysUntilMonday = (8 - now.getDay()) % 7 || 7
  const nextMonday = new Date(now)
  nextMonday.setDate(now.getDate() + daysUntilMonday)
  return `Next update ${nextMonday.toLocaleDateString([], { weekday: 'long', month: 'short', day: 'numeric' })}`
}

export function TracklistDropdown({ playlist, lbUser }) {
  const [tracks, setTracks] = useState([])
  const [generatedAt, setGeneratedAt] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [fetching, setFetching] = useState(false)

  const loadTracks = (withRetry = false) => {
    let cancelled = false
    let retry = 0
    let retryTimer = null
    setLoading(true)
    setError(null)
    const load = () => {
      fetchPlaylistTracks(playlist, { force: retry > 0 || withRetry })
        .then(({ tracks: t, generatedAt: g }) => {
          if (cancelled) return
          if (t.length === 0 && withRetry && retry < 8) {
            retry += 1
            retryTimer = setTimeout(load, 1500)
            return
          }
          setTracks(t)
          setGeneratedAt(g)
          setLoading(false)
        })
        .catch(e => { if (!cancelled) { setError(e.message); setLoading(false) } })
    }
    load()
    return () => { cancelled = true; if (retryTimer) clearTimeout(retryTimer) }
  }

  useEffect(() => {
    if (!playlist) return
    return loadTracks(false)
  }, [playlist])

  const handleFetch = () => {
    if (!lbUser) return
    setFetching(true)
    prefetchPlaylists(lbUser, [playlist])
      .then(() => loadTracks(true))
      .catch(e => setError(e.message))
      .finally(() => setFetching(false))
  }

  const genDate = generatedAt ? new Date(generatedAt) : null

  return (
    <div style={{ marginTop: 16 }}>
      {/* Header */}
      <div style={{ paddingBottom: 12, borderBottom: '1px solid #232323', display: 'flex', alignItems: 'baseline', gap: 10 }}>
        <span style={{ fontSize: 11, color: '#ffffff', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          {!loading && tracks.length ? `${tracks.length} Tracks` : 'Tracks'}
        </span>
        {!loading && genDate && (
          <span style={{ fontSize: 10, color: '#565656' }}>
            Generated {genDate.toLocaleDateString([], { month: 'short', day: 'numeric' })}
          </span>
        )}
      </div>

      {/* Track list */}
      <div className="no-scrollbar" style={{ maxHeight: 560, overflowY: 'auto', scrollbarWidth: 'none', msOverflowStyle: 'none' }}>
        {loading ? (
          <div style={{ padding: '16px 2px', fontSize: 12, color: '#4a4a4a' }}>{fetching ? 'Fetching…' : 'Loading…'}</div>
        ) : error ? (
          <div style={{ padding: '16px 2px', fontSize: 12, color: '#c0392b' }}>{error}</div>
        ) : tracks.length === 0 ? (
          <div style={{ padding: '16px 2px', fontSize: 12, color: '#4a4a4a', display: 'flex', alignItems: 'center', gap: 10 }}>
            <span>No playlist found yet. {nextUpdateLabel(playlist)}.</span>
            {lbUser && (
              <button
                onClick={handleFetch}
                disabled={fetching}
                style={{
                  fontSize: 11, padding: '3px 10px', borderRadius: 5, border: '1px solid #333',
                  background: '#1f1f1f', color: '#aaa', cursor: 'pointer', flexShrink: 0,
                }}
              >
                Pull tracks
              </button>
            )}
          </div>
        ) : (
          tracks.map(t => (
            <TrackRow key={`${t.rank}-${t.title}-${t.artist}`} track={t} />
          ))
        )}
      </div>
    </div>
  )
}

// ── PlaylistCard ──────────────────────────────────────────────────────────────

const withAlpha = (hex, alpha) => `${hex}${Math.round(alpha * 255).toString(16).padStart(2, '0')}`

const cardGradient = (shadow, midtone, highlight, base = shadow) => `
  linear-gradient(135deg, ${withAlpha(shadow, 0.36)} 0%, ${withAlpha(midtone, 0.28)} 48%, ${withAlpha(highlight, 0.18)} 100%),
  ${base}`

const PRESETS = {
  'weekly-exploration': {
    background: cardGradient('#2979ff', '#7c3aed', '#0ea5e9'),
    accent: '#818cf8',
    label: 'WEEKLY',
  },
  'weekly-jams': {
    background: cardGradient('#fb923c', '#ef4444', '#f472b6'),
    accent: '#fb923c',
    label: 'WEEKLY',
  },
  'daily-jams': {
    background: cardGradient('#10b981', '#06b6d4', '#22c55e'),
    accent: '#34d399',
    label: 'DAILY',
  },
  'on-repeat': {
    background: cardGradient('#e11d48', '#9f1239', '#fb7185'),
    accent: '#fb7185',
    label: 'MONTHLY',
  },
}

const FALLBACK = {
  background: cardGradient('#646478', '#50506e', '#828296'),
  accent: '#b3b3b3',
  label: 'PLAYLIST',
}

const SCHEDULE_DAYS = [
  { value: -1,  label: 'Every day' },
  { value: 0,   label: 'Sunday' },
  { value: 1,   label: 'Monday' },
  { value: 2,   label: 'Tuesday' },
  { value: 3,   label: 'Wednesday' },
  { value: 4,   label: 'Thursday' },
  { value: 5,   label: 'Friday' },
  { value: 6,   label: 'Saturday' },
  { value: 100, label: 'Monthly (1st)' },
]

// Inline SVG noise — subtle film-grain texture
const NOISE = `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='256' height='256'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`

export function PlaylistCard({
  playlist,
  schedule: s,
  locked,
  fixedSchedule = false,
  index = 0,
  nextRunText,
  scheduleSaveStatus,
  onToggle,
  onToggleEdit,
  onSave,
  onCancelEdit,
  onDayChange,
  onTimeChange,
  gradient: gradientOverride,
  tracklistOpen,
  onTracklistToggle,
}) {
  const { value, name } = playlist
  const preset = PRESETS[value] ?? FALLBACK
  const bg = gradientOverride ?? preset.background
  const { accent, label } = preset

  // Split gradient string into radial layers + solid base color so album art can sit between them.
  const bgLines = bg.trim().split('\n').map(l => l.trim()).filter(Boolean)
  const lastLine = bgLines[bgLines.length - 1]
  const isBase = /^#[0-9a-fA-F]{3,8}$/.test(lastLine) || /^rgba?\(/.test(lastLine)
  const gradientLayers = (isBase ? bgLines.slice(0, -1) : bgLines)
    .map(l => l.replace(/,\s*$/, ''))
    .join(',\n')
  const baseColor = isBase ? lastLine : '#000'

  // Album art slideshow
  const [bgCovers, setBgCovers] = useState([])
  const [coverIdx, setCoverIdx] = useState(0)

  useEffect(() => {
    if (!s.enabled) return
    let cancelled = false
    let retry = 0
    let retryTimer = null
    const load = () => {
      fetchPlaylistTracks(value, { force: retry > 0 })
        .then(({ tracks }) => {
          if (cancelled) return
          const covers = tracks.map(t => t.coverUrl).filter(Boolean)
          if (covers.length === 0 && retry < 8) {
            retry += 1
            retryTimer = setTimeout(load, 1500)
            return
          }
          setBgCovers(covers)
        })
        .catch(() => {})
    }
    load()
    return () => {
      cancelled = true
      if (retryTimer) clearTimeout(retryTimer)
    }
  }, [value, s.enabled])

  useEffect(() => {
    if (bgCovers.length < 2) return
    const id = setInterval(() => setCoverIdx(i => (i + 1) % bgCovers.length), 5000)
    return () => clearInterval(id)
  }, [bgCovers.length])

  const [line1, ...rest] = name.split(' ')
  const line2 = rest.join(' ')
  const timeStr = `${String(s.hour).padStart(2, '0')}:${String(s.minute).padStart(2, '0')}`

  return (
    <motion.div
      initial={{ opacity: 0, y: 28, scale: 0.96 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ duration: 0.55, delay: index * 0.09, ease: [0.16, 1, 0.3, 1] }}
    >
      <div
        onClick={s.enabled ? onTracklistToggle : undefined}
        className="playlist-card"
        style={{
          position: 'relative',
          borderRadius: 8,
          overflow: 'hidden',
          isolation: 'isolate',
          background: baseColor,
          filter: s.enabled ? 'none' : 'grayscale(1) brightness(0.55)',
          transition: 'filter 0.4s ease, box-shadow 0.35s ease',
          boxShadow: tracklistOpen
            ? '0 8px 32px rgba(0,0,0,0.55), inset 0 1px 0 rgba(255,255,255,0.08), 0 0 0 1px rgba(255,255,255,0.08)'
            : s.enabled
              ? '0 8px 32px rgba(0,0,0,0.55), inset 0 1px 0 rgba(255,255,255,0.08)'
              : '0 4px 16px rgba(0,0,0,0.35)',
          cursor: s.enabled ? 'pointer' : 'default',
        }}
      >
        {/* Gradient map color field */}
        <div style={{ position: 'absolute', inset: 0, backgroundImage: gradientLayers }} />

        {/* Album art luminosity — gives the gradient field cover-art detail */}
        <AnimatePresence>
          {bgCovers[coverIdx] && (
            <motion.img
              key={coverIdx}
              src={bgCovers[coverIdx]}
              alt=""
              initial={{ opacity: 0 }}
              animate={{ opacity: 0.86 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 1.5, ease: 'easeInOut' }}
              onError={() => setBgCovers(prev => prev.filter((_, i) => i !== coverIdx))}
              style={{
                position: 'absolute', inset: 0,
                width: '100%', height: '100%',
                objectFit: 'cover', display: 'block',
                filter: 'grayscale(1) contrast(1) brightness(0.5)',
                mixBlendMode: 'luminosity',
              }}
            />
          )}
        </AnimatePresence>

        {/* Black wash — tune opacity to control gradient-map visibility */}
        <div style={{ position: 'absolute', inset: 0, background: '#000', opacity: 0.38 }} />

        {/* Noise overlay */}
        <div style={{
          position: 'absolute', inset: 0,
          backgroundImage: NOISE,
          backgroundSize: '256px 256px',
          opacity: 0.045,
          mixBlendMode: 'overlay',
        }} />

        {/* Bottom vignette — keeps controls legible */}
        <div style={{
          position: 'absolute', bottom: 0, left: 0, right: 0, height: '55%',
          background: 'linear-gradient(0deg, rgba(0,0,0,0.72) 0%, rgba(0,0,0,0.2) 60%, transparent 100%)',
        }} />

        {label && (
          <div style={{
            position: 'absolute', top: 7, left: 8,
            fontSize: 12, fontWeight: 700, letterSpacing: '0.10em',
            color: 'white',
            mixBlendMode: 'overlay',
          }}>
            {label}
          </div>
        )}


        {/* Name + schedule — bottom left */}
        <div style={{ position: 'absolute', bottom: 8, left: 10, maxWidth: 'calc(100% - 50px)', minWidth: 0 }}>
          <div style={{
            fontFamily: "'Bebas Neue', sans-serif",
            lineHeight: 0.95,
            color: 'white',
            textShadow: '0 1px 8px rgba(0,0,0,0.5)',
            letterSpacing: '0.025em',
          }}>
            <div style={{ fontSize: 'clamp(11px, 3.5vw, 20px)', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{line1}</div>
            {line2 && <div style={{ fontSize: 'clamp(11px, 3.5vw, 20px)', opacity: 0.88, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{line2}</div>}
          </div>
          <span
            onClick={e => { e.stopPropagation(); if (!locked && !fixedSchedule) onToggleEdit() }}
            style={{
              display: 'block', marginTop: 4,
              fontSize: 8, fontWeight: 300,
              color: 'rgba(255,255,255,0.55)',
              mixBlendMode: 'hard-light',
              cursor: (locked || fixedSchedule) ? 'default' : 'pointer',
              letterSpacing: '0.02em',
              whiteSpace: 'nowrap',
              transition: 'color 0.3s',
            }}
          >
            {nextRunText}
          </span>
        </div>

        {/* Toggle — bottom right */}
        <label
          onClick={e => e.stopPropagation()}
          style={{
            position: 'absolute', bottom: 8, right: 8,
            display: 'flex', alignItems: 'center',
            cursor: locked ? 'not-allowed' : 'pointer',
            opacity: locked ? 0.5 : 1,
          }}
        >
          <Toggle checked={s.enabled} onChange={onToggle} disabled={locked} tiny />
        </label>

        {locked && (
          <span style={{
            position: 'absolute', bottom: 10, right: 30,
            fontSize: 7, letterSpacing: '0.12em', color: 'rgba(255,255,255,0.3)',
          }}>ENV</span>
        )}
      </div>

      {/* Inline schedule editor */}
      <AnimatePresence>
        {s.editing && s.enabled && !locked && !fixedSchedule && (
          <motion.div
            key="editor"
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.22, ease: 'easeInOut' }}
            style={{ overflow: 'hidden' }}
          >
            <div style={{
              marginTop: 8,
              background: '#161616', border: '1px solid #2a2a2a',
              borderRadius: 10, padding: '11px 13px',
              display: 'flex', flexDirection: 'column', gap: 9,
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <span style={{ fontSize: 12, color: '#7a7a7a' }}>Runs</span>
                <select
                  value={s.day}
                  onChange={e => onDayChange(parseInt(e.target.value))}
                  style={{
                    background: '#1f1f1f', border: '1px solid #333', color: 'white',
                    borderRadius: 6, padding: '5px 10px', fontSize: 13, cursor: 'pointer', outline: 'none',
                  }}
                >
                  {SCHEDULE_DAYS.map(d => <option key={d.value} value={d.value}>{d.label}</option>)}
                </select>
                <span style={{ fontSize: 12, color: '#7a7a7a' }}>at</span>
                <input
                  type="time"
                  value={timeStr}
                  onChange={e => onTimeChange(e.target.value)}
                  style={{
                    background: '#1f1f1f', border: '1px solid #333', color: 'white',
                    borderRadius: 6, padding: '5px 8px', fontSize: 13, outline: 'none',
                  }}
                />
              </div>
              <div style={{ display: 'flex', gap: 7 }}>
                <Button style={{ padding: '4px 12px', fontSize: 12 }} onClick={onSave}>Save</Button>
                <Button style={{ padding: '4px 10px', fontSize: 12 }} onClick={onCancelEdit}>✕</Button>
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}
