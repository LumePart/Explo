/**
 * Settings.jsx
 *
 * Main app view after initial setup. Three tabs: Home, Settings, Logs.
 * Each section fetches its own data directly from the API — no prop-drilling.
 *
 * Sections:
 *   HomeSection    — scheduled playlists, manual run, live output
 *   ConfigSection  — raw .env editor, wizard re-run, reset
 *   LogsSection    — full server log viewer
 */

import { useState, useEffect, useCallback, useRef } from 'react'
import {
  fetchConfig, fetchConfigRaw, saveConfig, resetConfig,
  saveSchedule, startRun, stopRun, fetchRunStatus, fetchLogs,
  fetchCustomPlaylists, deleteCustomPlaylist,
} from '../lib/api'
import { parseSlogLine, cronToFields, highlightEnv } from '../lib/utils'
import { fetchPlaylistTracks } from '../lib/listenbrainz'
import { motion, AnimatePresence } from 'motion/react'
import { Toggle } from './ui/Toggle'
import { Button, SectionLabel, Panel, LogRow } from './ui/common'
import { PlaylistCard, TracklistDropdown } from './ui/PlaylistCard'
import { ImportModal } from './ui/ImportModal'
import { UpdateNotification } from './ui/UpdateNotification'

const tabBtnCls = active =>
  `bg-transparent border-none border-b-2 pb-2 px-3.5 text-[13px] leading-none cursor-pointer transition-colors
  ${active ? 'text-accent border-accent' : 'text-muted border-transparent hover:text-white'}`

// ── Home Tab ──────────────────────────────────────────────────────────────────
// Manages scheduled playlists, manual runs, and live run output.
// Fetches its own config on mount to initialise schedule state and locked keys.

// Streams live run output from /api/ui/run/events
function useSSE({ onLine, onDone }) {
  const abortRef = useRef(null)

  const connect = useCallback(async () => {
    if (abortRef.current) abortRef.current.abort()
    const controller = new AbortController()
    abortRef.current = controller
    try {
      const res = await fetch('/api/ui/run/events', { credentials: 'include', signal: controller.signal })
      if (!res.ok) { onDone(null); return }
      const reader = res.body.getReader()
      const dec = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })
        const parts = buf.split('\n\n')
        buf = parts.pop()
        for (const part of parts) {
          let ev = '', data = ''
          for (const l of part.split('\n')) {
            if (l.startsWith('event: ')) ev = l.slice(7).trim()
            if (l.startsWith('data: ')) data = l.slice(6)
          }
          if (ev === 'done') { onDone(parseInt(data)); return }
          else if (data) onLine(data)
        }
      }
    } catch (e) {
      if (e.name !== 'AbortError') onDone(null)
    } finally {
      if (abortRef.current === controller) abortRef.current = null
    }
  }, [onLine, onDone])

  const disconnect = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
  }, [])

  return { connect, disconnect }
}

const PLAYLISTS = [
  { value: 'weekly-exploration', name: 'Weekly Exploration', scheduleKey: 'WEEKLY_EXPLORATION_SCHEDULE', defaultDay: 2,  defaultHour: 0,  defaultMinute: 15 },
  { value: 'weekly-jams',        name: 'Weekly Jams',        scheduleKey: 'WEEKLY_JAMS_SCHEDULE',        defaultDay: 1,  defaultHour: 0,  defaultMinute: 30 },
  { value: 'daily-jams',         name: 'Daily Jams',         scheduleKey: 'DAILY_JAMS_SCHEDULE',         defaultDay: -1, defaultHour: 1,  defaultMinute: 15 },
  { value: 'on-repeat',          name: 'On Repeat',          scheduleKey: 'ON_REPEAT_SCHEDULE',          defaultDay: 100, defaultHour: 12, defaultMinute: 0, fixedSchedule: true },
]

const SCHEDULE_DAYS = [
  { value: -2,   label: 'Never',           summary: 'No schedule' },
  { value: -1,   label: 'Every day',       summary: 'Every day' },
  { value: 0,    label: 'Sunday',          summary: 'Every Sunday' },
  { value: 1,    label: 'Monday',          summary: 'Every Monday' },
  { value: 2,    label: 'Tuesday',         summary: 'Every Tuesday' },
  { value: 3,    label: 'Wednesday',       summary: 'Every Wednesday' },
  { value: 4,    label: 'Thursday',        summary: 'Every Thursday' },
  { value: 5,    label: 'Friday',          summary: 'Every Friday' },
  { value: 6,    label: 'Saturday',        summary: 'Every Saturday' },
  { value: 100,  label: 'Monthly (1st)',   summary: 'Every 1st of the month' },
]

const selectCls = 'bg-surface border border-ui-border text-white rounded-[6px] px-2.5 py-1.5 text-[13px] cursor-pointer outline-none focus:border-accent'

function TracklistSlide({ show, slideKey, children }) {
  return (
    <AnimatePresence>
      {show && (
        <motion.div
          key={slideKey}
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: 'auto' }}
          exit={{ opacity: 0, height: 0 }}
          transition={{ duration: 0.28, ease: 'easeInOut' }}
          style={{ overflow: 'hidden' }}
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function CustomPlaylistsSection({
  customPlaylists,
  schedules,
  scheduleProps,
  openTracklist,
  setOpenTracklist,
  lbUser,
  onImportedRefresh,
  onSync,
  onDelete,
  showImportModal,
  setShowImportModal,
}) {
  return (
    <div className="mt-6">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div className="text-[16px] font-bold tracking-tight text-white">Custom Playlists</div>
        <button
          onClick={() => setShowImportModal(true)}
          className="bg-transparent border border-ui-border text-muted rounded-full px-3 py-1 text-[12px] cursor-pointer hover:text-white hover:border-[#444] transition-colors"
        >
          + Import
        </button>
      </div>

      {customPlaylists.length === 0 ? (
        <p className="text-[12px] text-muted mt-3">
          No custom playlists yet. Import one from ListenBrainz or Apple Music.
        </p>
      ) : (
        <div className="flex gap-3 mt-3 overflow-x-auto snap-x snap-mandatory pb-2">
          {customPlaylists.map((cp, i) => {
            if (!schedules[cp.id]) return null
            return (
              <div
                key={cp.id}
                className="shrink-0 snap-start w-full min-[420px]:w-[calc((100%-12px)/2)] min-[720px]:w-[calc((100%-36px)/4)]"
              >
                <PlaylistCard
                  playlist={{ value: `custom-${cp.color_index ?? i}`, name: cp.name }}
                  trackId={cp.id}
                  artworkUrl={cp.artwork_url || undefined}
                  {...scheduleProps(cp.id)}
                  index={i}
                  nextRunText={schedules[cp.id]?.enabled
                    ? SCHEDULE_DAYS.find(d => d.value === schedules[cp.id].day)?.summary ?? 'Every day'
                    : 'Disabled'}
                  tracklistOpen={openTracklist === cp.id}
                  onTracklistToggle={() => setOpenTracklist(v => v === cp.id ? null : cp.id)}
                  onDelete={(opts) => onDelete(cp.id, opts)}
                />
              </div>
            )
          })}
        </div>
      )}

      <TracklistSlide show={openTracklist && openTracklist.startsWith('custom-')} slideKey={openTracklist}>
        <TracklistDropdown
          playlist={openTracklist}
          lbUser={null}
          onRun={() => onSync(openTracklist)}
        />
      </TracklistSlide>

      <AnimatePresence>
        {showImportModal && (
          <ImportModal
            onClose={() => setShowImportModal(false)}
            onImported={onImportedRefresh}
            onSync={onSync}
          />
        )}
      </AnimatePresence>
    </div>
  )
}

function HomeSection() {
  const [schedules, setSchedules] = useState(null)
  const [envSources, setEnvSources] = useState({})
  const [scheduleSaveStatus, setScheduleSaveStatus] = useState({})
  const [lbUser, setLbUser] = useState('')
  const [openTracklist, setOpenTracklist] = useState(null)
  const [customPlaylists, setCustomPlaylists] = useState([])
  const [showImportModal, setShowImportModal] = useState(false)

  const [playlist, setPlaylist] = useState('weekly-exploration')
  const [dlmode, setDlmode] = useState('normal')
  const [noPersist, setNoPersist] = useState(false)
  const [excludeLocal, setExcludeLocal] = useState(false)

  const [running, setRunning] = useState(false)
  const [status, setStatus] = useState('')
  const [logEntries, setLogEntries] = useState([])
  const [rawLog, setRawLog] = useState(false)
  const logRef = useRef(null)

  useEffect(() => {
    Promise.all([
      fetchConfig(),
      fetchCustomPlaylists().catch(() => [])
    ]).then(([{ values, sources }, customList]) => {
      setEnvSources(sources || {})
      setLbUser(values.LISTENBRAINZ_USER || '')
      setCustomPlaylists(customList)

      const s = {}
      for (const p of PLAYLISTS) {
        const cron = values[p.scheduleKey]
        s[p.value] = cron
          ? { enabled: true, editing: false, ...cronToFields(cron) }
          : { enabled: false, day: p.defaultDay, hour: p.defaultHour, minute: p.defaultMinute, editing: false }
      }
      for (const cp of customList) {
        s[cp.id] = cp.schedule
          ? { enabled: true, editing: false, ...cronToFields(cp.schedule) }
          : cp.flags
            ? { enabled: true, editing: false, day: -2, hour: 4, minute: 0 }
            : { enabled: false, day: -1, hour: 4, minute: 0, editing: false }
      }
      setSchedules(s)
    })
  }, [])

  const onLine = useCallback(data => {
    setLogEntries(prev => [...prev, { raw: data, ...parseSlogLine(data) }])
    requestAnimationFrame(() => {
      if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
    })
  }, [])

  const onDone = useCallback(code => {
    setStatus(code === 0 ? 'done ✓' : code === null ? 'error' : `failed (exit ${code})`)
    setRunning(false)
  }, [])

  const { connect, disconnect } = useSSE({ onLine, onDone })

  useEffect(() => {
    fetchRunStatus().then(s => {
      if (s.running) {
        setRunning(true)
        setStatus('running…')
        setLogEntries([])
        connect()
      }
    })
    return () => disconnect()
  }, [connect, disconnect])

  const isScheduleLocked = id => {
    const p = PLAYLISTS.find(p => p.value === id)
    return p ? envSources[p.scheduleKey] === 'env' : false
  }

  const nextRunText = id => {
    const s = schedules[id]
    if (!s?.enabled) return 'Disabled'
    return SCHEDULE_DAYS.find(d => d.value === s.day)?.summary || 'Every day'
  }

  const flashStatus = (id, msg) => {
    setScheduleSaveStatus(prev => ({ ...prev, [id]: msg }))
    if (msg === 'Saved.') setTimeout(() => setScheduleSaveStatus(prev => ({ ...prev, [id]: '' })), 2000)
  }

  const scheduleProps = id => {
    const s = schedules[id]
    return {
      schedule: s,
      scheduleSaveStatus: scheduleSaveStatus[id] || '',
      onToggle: v => {
        setSchedules(prev => ({ ...prev, [id]: { ...prev[id], enabled: v } }))
        saveSchedule(id, v, s.day, s.hour, s.minute)
          .then(() => flashStatus(id, 'Saved.'))
          .catch(() => flashStatus(id, 'Error saving.'))
      },
      onToggleEdit: () => setSchedules(prev => ({
        ...prev, [id]: { ...prev[id], editing: !prev[id].editing }
      })),
      onSave: () => {
        if (isScheduleLocked(id)) return
        saveSchedule(id, s.enabled, s.day, s.hour, s.minute)
          .then(() => flashStatus(id, 'Saved.'))
          .catch(() => flashStatus(id, 'Error saving.'))
        setSchedules(prev => ({ ...prev, [id]: { ...prev[id], editing: false } }))
      },
      onCancelEdit: () => setSchedules(prev => ({
        ...prev, [id]: { ...prev[id], editing: false }
      })),
      onDayChange: day => setSchedules(prev => ({
        ...prev, [id]: { ...prev[id], day }
      })),
      onTimeChange: val => {
        const [h = '00', m = '00'] = val.split(':')
        setSchedules(prev => ({
          ...prev, [id]: { ...prev[id], hour: parseInt(h) || 0, minute: parseInt(m) || 0 }
        }))
      },
    }
  }

  const handleRun = async () => {
    setRunning(true)
    setLogEntries([])
    setStatus('running…')
    try {
      await startRun(playlist, dlmode, !noPersist, excludeLocal)
      connect()
    } catch (e) {
      if (e.conflict) { setStatus('already running'); setRunning(false); return }
      setStatus('error')
      setRunning(false)
    }
  }

  const handleStop = async () => {
    setStatus('stopping…')
    try { await stopRun() }
    catch { setStatus('error stopping run') }
  }

  if (!schedules) return null

  return (
    <div>
      {/* Scheduled Playlists */}
      <div className="mt-6">
        <div className="text-[16px] font-bold tracking-tight text-white mb-3.5">Scheduled Playlists</div>
        <div className="grid grid-cols-1 min-[420px]:grid-cols-2 min-[720px]:grid-cols-4 gap-3 mt-3">
          {PLAYLISTS.map((p, i) => (
            <PlaylistCard
              key={p.value}
              playlist={p}
              {...scheduleProps(p.value)}
              locked={isScheduleLocked(p.value)}
              fixedSchedule={!!p.fixedSchedule}
              index={i}
              nextRunText={nextRunText(p.value)}
              tracklistOpen={openTracklist === p.value}
              onTracklistToggle={() => setOpenTracklist(v => v === p.value ? null : p.value)}
            />
          ))}
        </div>
        <TracklistSlide show={openTracklist && PLAYLISTS.some(p => p.value === openTracklist)} slideKey={openTracklist}>
          <TracklistDropdown lbUser={lbUser} playlist={openTracklist} />
        </TracklistSlide>
        <p className="text-[12px] text-muted mt-3">Schedule changes take effect after restarting the container.</p>
      </div>

      {/* Custom Playlists */}
      <CustomPlaylistsSection
        customPlaylists={customPlaylists}
        schedules={schedules}
        scheduleProps={scheduleProps}
        openTracklist={openTracklist}
        setOpenTracklist={setOpenTracklist}
        lbUser={lbUser}
        showImportModal={showImportModal}
        setShowImportModal={setShowImportModal}
        onImportedRefresh={() => {
          fetchCustomPlaylists().then(list => {
            setCustomPlaylists(list)
            setSchedules(prev => {
              const next = { ...prev }
              for (const cp of list) {
                if (next[cp.id]) continue
                next[cp.id] = cp.schedule
                  ? { enabled: true, editing: false, ...cronToFields(cp.schedule) }
                  : cp.flags
                    ? { enabled: true, editing: false, day: -2, hour: 4, minute: 0 }
                    : { enabled: false, day: -1, hour: 4, minute: 0, editing: false }
              }
              return next
            })
          }).catch(() => {})
          setShowImportModal(false)
        }}
        onSync={async (id) => {
          await startRun(id, 'normal', true, false)
          setRunning(true)
          setStatus('running…')
          setLogEntries([])
          connect()
        }}
        onDelete={async (id, opts) => {
          try {
            await deleteCustomPlaylist(id, opts)
            setCustomPlaylists(prev => prev.filter(p => p.id !== id))
            setSchedules(prev => { const next = { ...prev }; delete next[id]; return next })
            if (openTracklist === id) setOpenTracklist(null)
          } catch {}
        }}
      />

      {/* Manual Run */}
      <div className="mt-6">
        <div className="text-[16px] font-bold tracking-tight text-white mb-3.5">Manual Run</div>
        <div className="flex flex-col gap-1.5 mb-3">
          <label className="text-[12px] text-muted">Download mode</label>
          <div className="flex gap-1.5">
            {[
              { value: 'normal', name: 'Normal', desc: "Download only if the track isn't found locally" },
              { value: 'skip',   name: 'Skip',   desc: 'No downloads — builds a playlist from tracks already in your library. Good for testing.' },
              { value: 'force',  name: 'Force',  desc: 'Always download, ignoring local tracks' },
            ].map(m => (
              <button
                key={m.value}
                onClick={() => setDlmode(m.value)}
                className={`px-3 py-1.5 text-[12px] rounded-[6px] border bg-surface cursor-pointer transition-colors
                  ${dlmode === m.value ? 'border-accent text-accent' : 'border-ui-border text-muted hover:border-[#404040] hover:text-white'}`}
              >
                {m.name}
              </button>
            ))}
          </div>
          <p className="text-[11px] text-muted">
            {({ normal: "Download only if the track isn't found locally", skip: 'No downloads — builds a playlist from tracks already in your library. Good for testing.', force: 'Always download, ignoring local tracks' })[dlmode]}
          </p>
        </div>
        <div className="flex gap-2.5 items-center flex-wrap mb-2.5">
          <label className="text-[12px] text-muted">Playlist</label>
          <select className={selectCls} value={playlist} onChange={e => setPlaylist(e.target.value)}>
            {PLAYLISTS.map(p => <option key={p.value} value={p.value}>{p.name}</option>)}
            {customPlaylists.length > 0 && <option disabled>---</option>}
            {customPlaylists.map(cp => <option key={cp.id} value={cp.id}>{cp.name}</option>)}
          </select>
          <label className="flex items-center gap-1.5 text-[12px] text-muted cursor-pointer" title="When unchecked (default), previously generated playlists and their tracks are kept and added to over time. When checked, the playlist is wiped and rebuilt from scratch on each run.">
            <input type="checkbox" checked={noPersist} onChange={e => setNoPersist(e.target.checked)} /> don't persist
          </label>
          <label className="flex items-center gap-1.5 text-[12px] text-muted cursor-pointer" title="When checked, tracks already found in your local library are excluded from the generated playlist.">
            <input type="checkbox" checked={excludeLocal} onChange={e => setExcludeLocal(e.target.checked)} /> exclude local
          </label>
        </div>
        <div className="flex gap-2.5 items-center">
          <Button onClick={handleRun} disabled={running}>▶ Run</Button>
          {running && (
            <button
              onClick={handleStop}
              className="bg-transparent border border-danger text-danger rounded-[6px] px-[18px] py-[7px] text-[13px] cursor-pointer hover:bg-danger hover:text-white transition-colors"
            >
              ■ Stop
            </button>
          )}
          <span className="text-[12px] text-muted">{status}</span>
        </div>
      </div>

      {/* Output */}
      <div className="mt-6">
        <div className="flex items-center justify-between mb-3.5">
          <SectionLabel className="">Output</SectionLabel>
          <label className="flex items-center gap-1.5 text-[12px] text-muted cursor-pointer">
            <input type="checkbox" checked={rawLog} onChange={e => setRawLog(e.target.checked)} /> Raw
          </label>
        </div>
        <Panel ref={logRef} className="w-full h-[300px]">
          {logEntries.map((e, i) => (
            rawLog
              ? <div key={i} className="font-mono text-[11px] text-accent whitespace-pre-wrap break-all py-px">{e.raw}</div>
              : <LogRow key={i} entry={e} />
          ))}
        </Panel>
      </div>
    </div>
  )
}

// ── Config Tab ────────────────────────────────────────────────────────────────
// Raw .env file viewer/editor, plus wizard re-run and full reset actions.
// Fetches its own raw config text from the API.

function ConfigSection({ onWizard }) {
  const [rawConfig, setRawConfig] = useState('')
  const [editing, setEditing] = useState(false)
  const [saveStatus, setSaveStatus] = useState('')

  useEffect(() => {
    fetchConfigRaw().then(text => setRawConfig(text))
  }, [])

  const handleSave = async () => {
    try {
      await saveConfig(rawConfig)
      setEditing(false)
      setSaveStatus('Saved.')
      setTimeout(() => setSaveStatus(''), 2500)
    } catch {
      setSaveStatus('Error saving.')
    }
  }

  const handleReset = async () => {
    if (!confirm('Reset all settings? This will restart the container and take you back to setup.')) return
    try {
      await resetConfig()
      const poll = async () => {
        try { await fetch('/api/config'); location.reload() }
        catch { setTimeout(poll, 1500) }
      }
      setTimeout(poll, 3000)
    } catch (e) {
      alert('Reset failed: ' + e.message)
    }
  }

  return (
    <div>
      <div className="mt-6">
        <div className="flex items-center justify-between mb-3.5">
          <SectionLabel className="">Config file</SectionLabel>
          {!editing ? (
            <Button onClick={() => setEditing(true)}>Edit</Button>
          ) : (
            <div className="flex items-center gap-2.5">
              <span className="text-[12px] text-muted">{saveStatus}</span>
              <Button onClick={handleSave}>Save</Button>
              <Button onClick={() => { fetchConfigRaw().then(setRawConfig); setEditing(false) }}>Cancel</Button>
            </div>
          )}
        </div>

        {!editing ? (
          <pre
            className="bg-well border border-ui-border rounded-[6px] w-full h-[420px] overflow-y-auto p-3.5 font-mono text-[12px] leading-relaxed whitespace-pre break-normal"
            dangerouslySetInnerHTML={{ __html: highlightEnv(rawConfig) }}
          />
        ) : (
          <textarea
            className="bg-well border border-ui-border text-white rounded-[6px] w-full h-[420px] p-3.5 font-mono text-[12px] leading-relaxed resize-y outline-none focus:border-accent"
            value={rawConfig}
            onChange={e => setRawConfig(e.target.value)}
            spellCheck={false}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
          />
        )}
      </div>

      <div className="mt-6">
        <SectionLabel>Setup</SectionLabel>
        <div className="flex flex-col items-start gap-2.5">
          <button
            onClick={onWizard}
            className="bg-transparent border-none text-muted text-[13px] cursor-pointer p-0 hover:text-white transition-colors"
          >
            Re-run setup wizard →
          </button>
          <button
            onClick={handleReset}
            className="bg-transparent border-none text-[#c0392b] text-[13px] cursor-pointer p-0 hover:text-[#d65546] transition-colors"
          >
            Reset all settings
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Logs Tab ──────────────────────────────────────────────────────────────────
// Displays the full server log file. Fetches its own log data from the API.

function LogsSection() {
  const [logFileEntries, setLogFileEntries] = useState([])
  const panelRef = useRef(null)

  const loadLog = () => {
    fetchLogs().then(text => {
      setLogFileEntries(text.split('\n').filter(l => l.trim()).map(l => ({ raw: l, ...parseSlogLine(l) })))
    })
  }

  useEffect(() => { loadLog() }, [])

  useEffect(() => {
    if (panelRef.current) panelRef.current.scrollTop = panelRef.current.scrollHeight
  }, [logFileEntries])

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-3.5">
        <SectionLabel className="">Log</SectionLabel>
        <button
          onClick={loadLog}
          className="bg-transparent border-none text-muted text-[11px] cursor-pointer p-0 hover:text-white transition-colors"
        >
          Refresh
        </button>
      </div>

      {logFileEntries.length === 0 ? (
        <p className="text-[12px] text-muted py-1">No log output yet.</p>
      ) : (
        <Panel ref={panelRef} className="h-[400px]">
          {logFileEntries.map((e, i) => <LogRow key={i} entry={e} />)}
        </Panel>
      )}
    </div>
  )
}

// ── Settings ──────────────────────────────────────────────────────────────────
// Tab shell. Routes between Home, Settings, and Logs sections.

// Module-level cache so the picked cover survives component remounts.
let _bgCoverCache = null

export default function Settings({ onWizard, onLogout }) {
  const [activeTab, setActiveTab] = useState('run')
  const [bgCover, setBgCover] = useState(_bgCoverCache)

  useEffect(() => {
    if (_bgCoverCache) return
    Promise.all(['weekly-exploration', 'weekly-jams', 'daily-jams', 'on-repeat'].map(
      t => fetchPlaylistTracks(t).catch(() => ({ tracks: [] }))
    )).then(results => {
      const covers = results.flatMap(r => (r.tracks ?? []).map(t => t.coverUrl).filter(Boolean))
      if (covers.length) {
        _bgCoverCache = covers[Math.floor(Math.random() * covers.length)]
        setBgCover(_bgCoverCache)
      }
    })
  }, [])

  return (
    <div style={{ position: 'relative', minHeight: '100vh' }}>
      <UpdateNotification />
      {/* Page background art */}
      <div style={{ position: 'fixed', inset: 0, zIndex: 0, background: '#121212', overflow: 'hidden', willChange: 'transform' }}>
        <AnimatePresence>
          {bgCover && (
            <motion.img
              key={bgCover}
              src={bgCover}
              alt=""
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 2, ease: 'easeInOut' }}
              style={{
                position: 'absolute', inset: 0,
                width: '100%', height: '100%',
                objectFit: 'cover',
                filter: 'blur(90px) saturate(1.8) brightness(0.14)',
                transform: 'scale(1.15) translateZ(0)',
                willChange: 'opacity',
              }}
            />
          )}
        </AnimatePresence>
      </div>

      {/* Content */}
      <div style={{ position: 'relative', zIndex: 1 }} className="min-h-screen">
        <div className="max-w-[980px] mx-auto px-6 pb-12">
          <header className="flex items-baseline gap-4 pt-5 pb-0 border-b border-ui-border mb-6">
            <span className="text-[16px] leading-none font-bold tracking-tight text-accent">Explo</span>
            <nav className="flex gap-0.5 items-baseline flex-1">
              <button className={tabBtnCls(activeTab === 'run')} onClick={() => setActiveTab('run')}>Home</button>
              <button className={tabBtnCls(activeTab === 'config')} onClick={() => setActiveTab('config')}>Settings</button>
              <button className={tabBtnCls(activeTab === 'logs')} onClick={() => setActiveTab('logs')}>Logs</button>
            </nav>
            <button
              onClick={onLogout}
              className="pb-2 text-[12px] text-muted hover:text-white transition-colors cursor-pointer bg-transparent border-none"
            >
              Sign out
            </button>
          </header>

          {activeTab === 'run' && <HomeSection />}
          {activeTab === 'config' && <ConfigSection onWizard={onWizard} />}
          {activeTab === 'logs' && <LogsSection />}
        </div>
      </div>
    </div>
  )
}
