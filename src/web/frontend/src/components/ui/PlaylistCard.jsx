import { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { Toggle } from './Toggle'
import { Button } from './common'
import { fetchPlaylistTracks } from '../../lib/listenbrainz'
import { prefetchPlaylists } from '../../lib/api'

// ── TrackRow ──────────────────────────────────────────────────────────────────

function TrackRow({ track, index = 0 }) {
  const [imgFailed, setImgFailed] = useState(false)
  const [imgLoaded, setImgLoaded] = useState(false)

  return (
    <div
      className="track-row"
      style={{
        '--delay': `${index * 30}ms`,
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
        position: 'relative', width: 42, height: 42, borderRadius: 3, flexShrink: 0,
        background: '#1e1e1e', overflow: 'hidden',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        {track.coverUrl && !imgFailed ? (
          <>
            <img
              src={track.coverUrl}
              alt=""
              loading="lazy"
              style={{
                width: '100%', height: '100%', objectFit: 'cover', display: 'block',
                opacity: imgLoaded ? 1 : 0, transition: 'opacity 0.35s ease',
              }}
              onLoad={() => setImgLoaded(true)}
              onError={() => setImgFailed(true)}
            />
            <motion.div
              animate={{ backgroundPosition: ['200% 0', '-200% 0'], opacity: imgLoaded ? 0 : 1 }}
              transition={{ backgroundPosition: { duration: 1.2, repeat: Infinity, ease: 'linear' }, opacity: { duration: 0.35 } }}
              style={{
                position: 'absolute', inset: 0,
                background: 'linear-gradient(90deg, #1e1e1e 25%, #2e2e2e 50%, #1e1e1e 75%)',
                backgroundSize: '200% 100%',
              }}
            />
          </>
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

export function TracklistDropdown({ playlist, lbUser, onRun }) {
  const [tracks, setTracks] = useState([])
  const [generatedAt, setGeneratedAt] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [fetching, setFetching] = useState(false)
  const [running, setRunning] = useState(false)
  const [runStatus, setRunStatus] = useState('')

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

  const handleRun = async () => {
    if (!onRun || running) return
    setRunning(true)
    setRunStatus('')
    try {
      await onRun()
      setRunStatus('Started')
      setTimeout(() => setRunStatus(''), 3000)
    } catch (e) {
      setRunStatus(e.message || 'Error')
    } finally {
      setRunning(false)
    }
  }

  const genDate = generatedAt ? new Date(generatedAt) : null

  return (
    <div style={{ marginTop: 16 }}>
      {/* Header */}
      <div style={{ paddingBottom: 12, borderBottom: '1px solid #232323', display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ fontSize: 11, color: '#ffffff', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          {!loading && tracks.length ? `${tracks.length} Tracks` : 'Tracks'}
        </span>
        {!loading && genDate && (
          <span style={{ fontSize: 10, color: '#565656' }}>
            Generated {genDate.toLocaleDateString([], { month: 'short', day: 'numeric' })}
          </span>
        )}
        {onRun && (
          <span style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 10 }}>
            {runStatus && <span style={{ fontSize: 10, color: '#565656' }}>{runStatus}</span>}
            <button
              onClick={handleRun}
              disabled={running}
              style={{
                background: 'none', border: 'none', padding: 0,
                fontSize: 10, letterSpacing: '0.06em', textTransform: 'uppercase',
                color: running ? '#3a3a3a' : '#565656',
                cursor: running ? 'default' : 'pointer',
              }}
              onMouseEnter={e => { if (!running) e.currentTarget.style.color = 'white' }}
              onMouseLeave={e => { if (!running) e.currentTarget.style.color = '#565656' }}
            >
              {running ? 'Starting…' : '▶ Run'}
            </button>
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
          <div style={{ padding: '16px 2px', fontSize: 12, color: '#4a4a4a', display: 'flex', flexDirection: 'column', gap: 8 }}>
            <span>No playlist found yet. {nextUpdateLabel(playlist)}.</span>
            {lbUser && (
              <button
                onClick={handleFetch}
                disabled={fetching}
                className="bg-transparent border border-ui-border text-muted rounded-full px-3 py-1 text-[11px] cursor-pointer hover:text-white hover:border-[#444] transition-colors self-start disabled:opacity-50"
              >
                Pull tracks
              </button>
            )}
          </div>
        ) : (
          tracks.map((t, i) => (
            <TrackRow key={`${t.rank}-${t.title}-${t.artist}`} track={t} index={i} />
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

// Color pool for user-imported custom playlists (cycled by colorIndex % 4)
const CUSTOM_PRESETS = [
  { background: cardGradient('#6366f1', '#8b5cf6', '#a78bfa'), accent: '#a78bfa' },
  { background: cardGradient('#0891b2', '#0e7490', '#67e8f9'), accent: '#67e8f9' },
  { background: cardGradient('#d97706', '#b45309', '#fcd34d'), accent: '#fcd34d' },
  { background: cardGradient('#16a34a', '#15803d', '#4ade80'), accent: '#4ade80' },
]

const SCHEDULE_DAYS = [
  { value: -2,   label: 'Never' },
  { value: -1,   label: 'Every day' },
  { value: 0,    label: 'Sunday' },
  { value: 1,    label: 'Monday' },
  { value: 2,    label: 'Tuesday' },
  { value: 3,    label: 'Wednesday' },
  { value: 4,    label: 'Thursday' },
  { value: 5,    label: 'Friday' },
  { value: 6,    label: 'Saturday' },
  { value: 100,  label: 'Monthly (1st)' },
]

// ── ScheduleEditor ────────────────────────────────────────────────────────────

function ScheduleEditor({ schedule: s, onSave, onCancelEdit, onDayChange, onTimeChange }) {
  const timeStr = `${String(s.hour).padStart(2, '0')}:${String(s.minute).padStart(2, '0')}`
  const open = s.editing && s.enabled
  const isNever = s.day === -2
  return (
    <div style={{
      display: 'grid',
      gridTemplateRows: open ? '1fr' : '0fr',
      opacity: open ? 1 : 0,
      transition: 'grid-template-rows 0.22s ease-in-out, opacity 0.22s ease-in-out',
    }}>
      <div style={{ overflow: 'hidden' }}>
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
              {SCHEDULE_DAYS.map(d => (
                <option key={d.value} value={d.value}>{d.label}</option>
              ))}
            </select>
            {!isNever && (
              <>
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
              </>
            )}
          </div>
          <div style={{ display: 'flex', gap: 7 }}>
            <Button style={{ padding: '4px 12px', fontSize: 12 }} onClick={onSave}>Save</Button>
            <Button style={{ padding: '4px 10px', fontSize: 12 }} onClick={onCancelEdit}>✕</Button>
          </div>
        </div>
      </div>
    </div>
  )
}

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
  onDelete,
  trackId,
  artworkUrl,
}) {
  const { value, name } = playlist
  // trackFetchId: use real playlist ID (custom playlists) if provided, else fall back to value
  const trackFetchId = trackId ?? value
  // Resolve preset: built-in types → PRESETS, custom-N → CUSTOM_PRESETS[N % 3], else FALLBACK
  let preset
  if (PRESETS[value]) {
    preset = PRESETS[value]
  } else {
    const customMatch = value.match(/^custom-(\d+)$/)
    preset = customMatch ? CUSTOM_PRESETS[Number(customMatch[1]) % CUSTOM_PRESETS.length] : FALLBACK
  }
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
      fetchPlaylistTracks(trackFetchId, { force: retry > 0 })
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
  }, [trackFetchId, s.enabled])

  useEffect(() => {
    if (bgCovers.length < 2) return
    const id = setInterval(() => setCoverIdx(i => (i + 1) % bgCovers.length), 5000)
    return () => clearInterval(id)
  }, [bgCovers.length])

  const [line1, ...rest] = name.split(' ')
  const line2 = rest.join(' ')

  const [menuOpen, setMenuOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteTracksChecked, setDeleteTracksChecked] = useState(false)
  const [cardHovered, setCardHovered] = useState(false)
  const canEdit = !locked && !fixedSchedule && !!onToggleEdit
  const hasMenu = canEdit || !!onDelete

  useEffect(() => {
    if (!menuOpen) { setConfirmDelete(false); setDeleteTracksChecked(false); return }
    const close = () => setMenuOpen(false)
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [menuOpen])

  return (
    <motion.div
      initial={{ opacity: 0, y: 28, scale: 0.96 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ duration: 0.55, delay: index * 0.09, ease: [0.16, 1, 0.3, 1] }}
      style={{ position: 'relative' }}
    >
      <div
        onClick={e => { if (menuOpen) return; if (s.enabled && onTracklistToggle) onTracklistToggle() }}
        onMouseEnter={() => setCardHovered(true)}
        onMouseLeave={() => setCardHovered(false)}
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

        {/* Playlist artwork — static cover image (e.g. Apple Music playlists) */}
        {artworkUrl && (
          <img
            src={artworkUrl}
            alt=""
            style={{
              position: 'absolute', inset: 0,
              width: '100%', height: '100%',
              objectFit: 'cover', display: 'block',
            }}
          />
        )}

        {/* Album art luminosity — gives the gradient field cover-art detail (skipped when static artwork present) */}
        {!artworkUrl && (
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
        )}

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

        {hasMenu && (
          <button
            onMouseDown={e => e.stopPropagation()}
            onClick={e => { e.stopPropagation(); setMenuOpen(o => !o) }}
            style={{
              position: 'absolute', top: 6, right: 8,
              background: 'none', border: 'none',
              color: 'white', fontSize: 18, lineHeight: 1,
              cursor: 'pointer', padding: '2px 6px',
              opacity: cardHovered || menuOpen ? 1 : 0.5,
              transition: 'opacity 0.15s',
              zIndex: 10,
            }}
          >
            ⋮
          </button>
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
          <span style={{
            display: 'block', marginTop: 4,
            fontSize: 8, fontWeight: 300,
            color: 'rgba(255,255,255,0.55)',
            mixBlendMode: 'hard-light',
            letterSpacing: '0.02em',
            whiteSpace: 'nowrap',
          }}>
            {nextRunText}
          </span>
        </div>

        {/* Toggle — bottom right */}
        {onToggle && (
          <>
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
          </>
        )}
      </div>

      {/* 3-dot dropdown menu */}
      <AnimatePresence>
      {menuOpen && hasMenu && (
        <motion.div
          onMouseDown={e => e.stopPropagation()}
          initial={{ opacity: 0, scale: 0.92, y: -6 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.94, y: -4 }}
          transition={{ duration: 0.16, ease: [0.16, 1, 0.3, 1] }}
          style={{
            position: 'absolute', top: 4, right: 4,
            zIndex: 50,
            transformOrigin: 'top right',
            background: '#1a1a1ae6',
            backdropFilter: 'blur(8px)',
            WebkitBackdropFilter: 'blur(8px)',
            border: '1px solid #282828',
            borderRadius: 8,
            padding: '4px 0',
            minWidth: 155,
            boxShadow: '0 8px 24px #00000088',
          }}
        >
          {canEdit && (
            <button
              onClick={e => { e.stopPropagation(); setMenuOpen(false); onToggleEdit() }}
              style={{
                display: 'block', width: '100%', textAlign: 'left',
                background: 'none', border: 'none',
                padding: '8px 14px', fontSize: 13, color: '#c0c0c0',
                cursor: 'pointer',
              }}
              onMouseEnter={e => { e.currentTarget.style.background = '#2a2a2a' }}
              onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
            >
              Edit Schedule
            </button>
          )}
          {onDelete && !confirmDelete && (
            <button
              onClick={e => { e.stopPropagation(); setConfirmDelete(true) }}
              style={{
                display: 'block', width: '100%', textAlign: 'left',
                background: 'none', border: 'none',
                padding: '8px 14px', fontSize: 13, color: '#e05050',
                cursor: 'pointer',
              }}
              onMouseEnter={e => { e.currentTarget.style.background = '#2a2a2a' }}
              onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
            >
              Delete Playlist
            </button>
          )}
          {onDelete && confirmDelete && (
            <div
              onMouseDown={e => e.stopPropagation()}
              style={{ padding: '8px 14px', display: 'flex', flexDirection: 'column', gap: 8 }}
            >
              <span style={{ fontSize: 12, color: '#9a9a9a' }}>Remove this playlist?</span>
              <label
                onClick={e => e.stopPropagation()}
                style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  fontSize: 11, color: '#9a9a9a', cursor: 'pointer',
                  userSelect: 'none',
                }}
              >
                <input
                  type="checkbox"
                  checked={deleteTracksChecked}
                  onChange={e => setDeleteTracksChecked(e.target.checked)}
                  style={{ margin: 0, cursor: 'pointer', accentColor: '#c0392b' }}
                />
                Also delete downloaded files
              </label>
              <div style={{ display: 'flex', gap: 6 }}>
                <button
                  onClick={e => { e.stopPropagation(); setMenuOpen(false); onDelete({ deleteTracks: deleteTracksChecked }) }}
                  style={{
                    flex: 1, background: '#6b1a1a', border: '1px solid #8b2a2a',
                    borderRadius: 5, padding: '5px 0', fontSize: 12,
                    color: '#ff8080', cursor: 'pointer',
                  }}
                >
                  Delete
                </button>
                <button
                  onClick={e => { e.stopPropagation(); setConfirmDelete(false); setDeleteTracksChecked(false) }}
                  style={{
                    flex: 1, background: '#242424', border: '1px solid #333',
                    borderRadius: 5, padding: '5px 0', fontSize: 12,
                    color: '#888', cursor: 'pointer',
                  }}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </motion.div>
      )}
      </AnimatePresence>

      {/* Inline schedule editor */}
      {!locked && !fixedSchedule && (
        <ScheduleEditor
          schedule={s}
          onSave={onSave}
          onCancelEdit={onCancelEdit}
          onDayChange={onDayChange}
          onTimeChange={onTimeChange}
        />
      )}
    </motion.div>
  )
}
