import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { importCustomPlaylist } from '../../lib/api'
import listenbrainzIcon from '../../assets/listenbrainz.svg'
import appleMusicIcon from '../../assets/apple-music.svg'
import spotifyIcon from '../../assets/spotify.svg'

const REFRESH_OPTIONS = [
  { value: 0,  label: 'Never' },
  { value: 1,  label: 'Every day' },
  { value: 7,  label: 'Every week' },
  { value: 14, label: 'Every 2 weeks' },
  { value: 30, label: 'Every month' },
]

const SOURCES = [
  { key: 'listenbrainz', label: 'ListenBrainz', icon: listenbrainzIcon, color: '#EB743B', placeholder: 'https://listenbrainz.org/playlist/\u2026' },
  { key: 'apple_music',  label: 'Apple Music',  icon: appleMusicIcon,  color: '#FA243C', placeholder: 'https://music.apple.com/us/playlist/\u2026' },
  { key: 'spotify',      label: 'Spotify',      icon: spotifyIcon,     color: '#1ed760', placeholder: 'https://open.spotify.com/playlist/\u2026' },
]

function CoverThumb({ src, index, onLoaded }) {
  const [loaded, setLoaded] = useState(false)
  const done = () => { setLoaded(true); onLoaded?.() }
  return (
    <div className="relative w-full aspect-square overflow-hidden" style={{ background: '#141414' }}>
      <motion.div
        animate={{ backgroundPosition: ['200% 0', '-200% 0'], opacity: loaded ? 0 : 1 }}
        transition={{
          backgroundPosition: { duration: 1.2, repeat: Infinity, ease: 'linear' },
          opacity: { duration: 0.4 },
        }}
        style={{
          position: 'absolute', inset: 0,
          background: 'linear-gradient(90deg, #141414 25%, #242424 50%, #141414 75%)',
          backgroundSize: '200% 100%',
          pointerEvents: 'none',
        }}
      />
      <motion.img
        src={src}
        alt=""
        onLoad={done}
        onError={done}
        initial={false}
        animate={loaded ? { opacity: 1, scale: 1 } : { opacity: 0, scale: 0.92 }}
        transition={{ duration: 1.2, delay: loaded ? index * 0.18 : 0, ease: [0.16, 1, 0.3, 1] }}
        className="w-full h-full object-cover block"
      />
    </div>
  )
}

function SuccessPanel({ result, onImported, onSync, onClose, onError }) {
  const unique = [...new Map((result.cover_urls ?? []).map(u => [u, u])).values()].slice(0, 6)
  const cols = unique.length <= 1 ? 1 : unique.length <= 4 ? 2 : 3
  const [loadedCount, setLoadedCount] = useState(0)
  const [footerReady, setFooterReady] = useState(unique.length === 0)

  const handleCoverLoaded = useCallback(() => setLoadedCount(n => n + 1), [])

  useEffect(() => {
    if (unique.length === 0 || loadedCount < unique.length) return
    const t = setTimeout(() => setFooterReady(true), (unique.length - 1) * 180 + 1300)
    return () => clearTimeout(t)
  }, [loadedCount, unique.length])

  return (
    <>
      <div className="flex items-center justify-between px-5 py-4 border-b border-ui-border">
        <div>
          <h2 className="text-[15px] font-semibold text-white leading-none">{result.name}</h2>
          <p className="text-[11px] text-muted mt-1">
            {result.track_count} track{result.track_count !== 1 ? 's' : ''} imported
          </p>
        </div>
      </div>

      {unique.length > 0 && (
        <div className="px-5 pt-4 pb-4">
          <div className={`grid gap-1.5 rounded-lg overflow-hidden ${
            cols === 1 ? 'grid-cols-1 max-w-[180px] mx-auto' : cols === 2 ? 'grid-cols-2' : 'grid-cols-3'
          }`}>
            {unique.map((src, i) => (
              <CoverThumb key={src} src={src} index={i} onLoaded={handleCoverLoaded} />
            ))}
          </div>
        </div>
      )}

      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: footerReady ? 1 : 0 }}
        transition={{ duration: 0.4 }}
        style={{ pointerEvents: footerReady ? 'auto' : 'none' }}
        className="flex justify-end gap-2 px-5 pb-5"
      >
        {onSync && (
          <button
            onClick={async () => {
              try { await onSync(result.id); onImported(result); onClose() }
              catch (e) { onError(e.message || 'Sync failed') }
            }}
            className="bg-transparent border border-ui-border text-muted rounded-full px-4 py-1.5 text-[13px] cursor-pointer hover:text-white hover:border-[#444] transition-colors"
          >
            Sync to Library
          </button>
        )}
        <button
          onClick={() => { onImported(result); onClose() }}
          className="bg-[var(--brand)] text-black border-none rounded-full px-5 py-1.5 text-[13px] font-semibold cursor-pointer hover:scale-[1.04] active:scale-[0.97] transition-transform"
        >
          Done
        </button>
      </motion.div>
    </>
  )
}

export function ImportModal({ onClose, onImported, onSync }) {
  const [source, setSource] = useState(null)   // 'listenbrainz' | 'apple_music'
  const [url, setUrl] = useState('')
  const [refreshDays, setRefreshDays] = useState(0)
  const [phase, setPhase] = useState('source') // 'source' | 'form' | 'success' | 'error'
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [errorMsg, setErrorMsg] = useState('')

  const sourceCfg = SOURCES.find(s => s.key === source)

  const handleImport = async () => {
    if (!url.trim() || loading) return
    setLoading(true)
    try {
      const data = await importCustomPlaylist(url.trim(), source, refreshDays)
      setResult(data)
      setPhase('success')
    } catch (e) {
      setErrorMsg(e.message || 'Import failed')
      setPhase('error')
    } finally {
      setLoading(false)
    }
  }

  const canSubmit = url.trim() && !loading

  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.2 }}
      onClick={(phase === 'source' || phase === 'form' || phase === 'error') && !loading ? onClose : undefined}
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{
        background: 'rgba(0, 0, 0, 0.72)',
        backdropFilter: 'blur(6px)',
        WebkitBackdropFilter: 'blur(6px)',
      }}
    >
      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.97 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        exit={{ opacity: 0, y: 16, scale: 0.97 }}
        transition={{ duration: 0.28, ease: [0.16, 1, 0.3, 1] }}
        onClick={e => e.stopPropagation()}
        className="w-full max-w-[420px] mx-4 border border-ui-border rounded-lg overflow-hidden"
        style={{
          background: '#1a1a1ae6',
          backdropFilter: 'blur(8px)',
          WebkitBackdropFilter: 'blur(8px)',
          boxShadow: '0 24px 64px #00000099',
          '--brand': sourceCfg?.color ?? '#1ed760',
        }}
      >
        <AnimatePresence mode="wait">

          {/* ── Source selection ─────────────────────────────── */}
          {phase === 'source' && (
            <motion.div
              key="source"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.15 }}
            >
              {/* Header */}
              <div className="flex items-center justify-between px-5 py-4 border-b border-ui-border">
                <h2 className="text-[15px] font-semibold text-white leading-none">
                  Choose a Playlist Source
                </h2>
                <button
                  onClick={onClose}
                  className="text-muted hover:text-white transition-colors text-[18px] leading-none p-0 bg-transparent border-none cursor-pointer"
                >
                  &times;
                </button>
              </div>

              {/* Source buttons */}
              <div className="px-5 pt-5 pb-5 grid grid-cols-3 gap-3">
                {SOURCES.map(s => (
                  <button
                    key={s.key}
                    onClick={() => { setSource(s.key); setPhase('form') }}
                    onMouseMove={e => {
                      const r = e.currentTarget.getBoundingClientRect()
                      e.currentTarget.style.setProperty('--mx', `${e.clientX - r.left}px`)
                      e.currentTarget.style.setProperty('--my', `${e.clientY - r.top}px`)
                    }}
                    style={{ '--brand': s.color }}
                    className="relative overflow-hidden aspect-square flex flex-col items-center justify-center gap-3 bg-well border border-ui-border rounded-md cursor-pointer transition-colors group hover:border-[var(--brand)]"
                  >
                    <div
                      aria-hidden
                      className="pointer-events-none absolute -inset-10 opacity-0 group-hover:opacity-100 transition-opacity duration-300"
                      style={{
                        background: 'radial-gradient(circle 230px at calc(var(--mx, 50%) + 40px) calc(var(--my, 50%) + 40px), var(--brand) 30%, transparent)',
                        filter: 'blur(22px)',
                      }}
                    />
                    <img
                      src={s.icon}
                      alt=""
                      draggable={false}
                      className="relative w-[40px] h-[40px] object-contain select-none transition-transform group-hover:scale-[1.04]"
                    />
                    <div className="relative text-[12px] font-bold tracking-tight text-white">
                      {s.label}
                    </div>
                  </button>
                ))}
              </div>
            </motion.div>
          )}

          {/* ── Form ───────────────────────────────────────── */}
          {phase === 'form' && (
            <motion.div
              key="form"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.15 }}
            >
              {/* Header */}
              <div className="flex items-center justify-between px-5 py-4 border-b border-ui-border">
                <h2 className="text-[15px] font-semibold text-white leading-none">
                  Import Playlist
                </h2>
                <button
                  onClick={onClose}
                  disabled={loading}
                  className="text-muted hover:text-white transition-colors text-[18px] leading-none p-0 bg-transparent border-none cursor-pointer disabled:opacity-40"
                >
                  &times;
                </button>
              </div>

              {/* Body */}
              <div className="px-5 pt-5 pb-5 flex flex-col gap-4">
                <div className="flex flex-col gap-1.5">
                  <label className="text-[12px] font-medium text-muted">
                    {sourceCfg?.label ?? 'Playlist'} URL
                  </label>
                  <input
                    type="text"
                    value={url}
                    onChange={e => setUrl(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleImport()}
                    placeholder={sourceCfg?.placeholder ?? 'Paste a playlist URL\u2026'}
                    autoFocus
                    disabled={loading}
                    className="w-full bg-well border border-ui-border text-white rounded-lg px-3 py-2 text-[13px] outline-none placeholder:text-[#444] focus:border-[var(--brand)] transition-colors disabled:opacity-50"
                  />
                </div>

                <div className="flex flex-col gap-1.5">
                  <label className="text-[12px] font-medium text-muted">
                    Auto-refresh
                  </label>
                  <select
                    value={refreshDays}
                    onChange={e => setRefreshDays(Number(e.target.value))}
                    disabled={loading}
                    className="bg-well border border-ui-border text-white rounded-lg px-3 py-2 text-[13px] cursor-pointer outline-none focus:border-[var(--brand)] transition-colors disabled:opacity-50"
                  >
                    {REFRESH_OPTIONS.map(o => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                </div>
              </div>

              {/* Footer */}
              <div className="flex justify-end gap-2 px-5 pb-5">
                <button
                  onClick={() => { setPhase('source'); setSource(null); setUrl('') }}
                  disabled={loading}
                  className="bg-transparent border border-ui-border text-muted rounded-full px-4 py-1.5 text-[13px] cursor-pointer hover:text-white hover:border-[#444] transition-colors disabled:opacity-40"
                >
                  Back
                </button>
                <button
                  onClick={handleImport}
                  disabled={!canSubmit}
                  className={`border-none rounded-full px-5 py-1.5 text-[13px] font-semibold transition-all
                    ${canSubmit
                      ? 'bg-[var(--brand)] text-black cursor-pointer hover:scale-[1.04] active:scale-[0.97]'
                      : 'bg-[#2a2a2a] text-[#555] cursor-default'
                    }`}
                >
                  {loading ? 'Importing\u2026' : 'Import'}
                </button>
              </div>
            </motion.div>
          )}

          {/* ── Success ────────────────────────────────────── */}
          {phase === 'success' && result && (
            <motion.div
              key="success"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
            >
              <SuccessPanel
                result={result}
                onImported={onImported}
                onSync={onSync}
                onClose={onClose}
                onError={msg => { setErrorMsg(msg); setPhase('error') }}
              />
            </motion.div>
          )}

          {/* ── Error ──────────────────────────────────────── */}
          {phase === 'error' && (
            <motion.div
              key="error"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.15 }}
            >
              {/* Header */}
              <div className="px-5 py-4 border-b border-ui-border">
                <h2 className="text-[15px] font-semibold text-white leading-none">
                  Import failed
                </h2>
                <p className="text-[12px] text-danger mt-1.5">{errorMsg}</p>
              </div>

              {/* Footer */}
              <div className="flex justify-end gap-2 px-5 py-4">
                <button
                  onClick={onClose}
                  className="bg-transparent border border-ui-border text-muted rounded-full px-4 py-1.5 text-[13px] cursor-pointer hover:text-white hover:border-[#444] transition-colors"
                >
                  Close
                </button>
                <button
                  onClick={() => { setPhase('form'); setErrorMsg('') }}
                  className="bg-[var(--brand)] text-black border-none rounded-full px-5 py-1.5 text-[13px] font-semibold cursor-pointer hover:scale-[1.04] active:scale-[0.97] transition-transform"
                >
                  Try again
                </button>
              </div>
            </motion.div>
          )}

        </AnimatePresence>
      </motion.div>
    </motion.div>
  )
}
