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
} from '../lib/api'
import { parseSlogLine, cronToFields, highlightEnv } from '../lib/utils'
import { Toggle } from './ui/Toggle'
import { Button, SectionLabel, Panel, LogRow } from './ui/common'

const tabBtnCls = active =>
  `bg-transparent border-none border-b-2 pb-2 px-3.5 text-[13px] cursor-pointer transition-colors relative top-px
  ${active ? 'text-white border-accent' : 'text-muted border-transparent hover:text-white'}`

// ── Home Tab ──────────────────────────────────────────────────────────────────
// Manages scheduled playlists, manual runs, and live run output.
// Fetches its own config on mount to initialise schedule state and locked keys.

// Streams live run output from /api/run/events
function useSSE({ onLine, onDone }) {
  const abortRef = useRef(null)

  const connect = useCallback(async () => {
    if (abortRef.current) abortRef.current.abort()
    const controller = new AbortController()
    abortRef.current = controller
    try {
      const res = await fetch('/api/run/events', { signal: controller.signal })
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
  { value: 'weekly-exploration', name: 'Weekly Exploration' },
  { value: 'weekly-jams',        name: 'Weekly Jams' },
  { value: 'daily-jams',         name: 'Daily Jams' },
]

const SCHEDULE_KEYS = {
  'weekly-exploration': 'WEEKLY_EXPLORATION_SCHEDULE',
  'weekly-jams':        'WEEKLY_JAMS_SCHEDULE',
  'daily-jams':         'DAILY_JAMS_SCHEDULE',
}

const SCHEDULE_DAYS = [
  { value: -1, label: 'Every day',   summary: 'Daily' },
  { value: 0,  label: 'Sunday',      summary: 'Every Sunday' },
  { value: 1,  label: 'Monday',      summary: 'Every Monday' },
  { value: 2,  label: 'Tuesday',     summary: 'Every Tuesday' },
  { value: 3,  label: 'Wednesday',   summary: 'Every Wednesday' },
  { value: 4,  label: 'Thursday',    summary: 'Every Thursday' },
  { value: 5,  label: 'Friday',      summary: 'Every Friday' },
  { value: 6,  label: 'Saturday',    summary: 'Every Saturday' },
]

const selectCls = 'bg-surface border border-ui-border text-white rounded-[6px] px-2.5 py-1.5 text-[13px] cursor-pointer outline-none focus:border-accent'

function initSchedules(config) {
  const defaults = {
    'weekly-exploration': { enabled: false, day: 2,  hour: 0, minute: 15, editing: false },
    'weekly-jams':        { enabled: false, day: 1,  hour: 0, minute: 30, editing: false },
    'daily-jams':         { enabled: false, day: -1, hour: 1, minute: 15, editing: false },
  }
  const out = {}
  for (const [name, def] of Object.entries(defaults)) {
    const cron = config[SCHEDULE_KEYS[name]]
    out[name] = cron ? { enabled: true, editing: false, ...cronToFields(cron) } : def
  }
  return out
}

function HomeSection() {
  const [schedules, setSchedules] = useState(null)
  const [envSources, setEnvSources] = useState({})
  const [scheduleSaveStatus, setScheduleSaveStatus] = useState({})

  const [playlist, setPlaylist] = useState('weekly-exploration')
  const [dlmode, setDlmode] = useState('normal')
  const [noPersist, setNoPersist] = useState(false)
  const [excludeLocal, setExcludeLocal] = useState(false)

  const [running, setRunning] = useState(false)
  const [status, setStatus] = useState('')
  const [logEntries, setLogEntries] = useState([])
  const [rawLog, setRawLog] = useState(false)
  const [recentTracks, setRecentTracks] = useState([])
  const logRef = useRef(null)

  useEffect(() => {
    fetchConfig().then(({ values, sources }) => {
      setSchedules(initSchedules(values))
      setEnvSources(sources || {})
    })
    fetchLogs().then(text => {
      const entries = text.split('\n').filter(l => l.trim()).map(l => ({ raw: l, ...parseSlogLine(l) }))
      setRecentTracks(entries.filter(e => e.track && e.level === 'INFO').reverse())
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

  const isScheduleLocked = name => envSources[SCHEDULE_KEYS[name]] === 'env'

  const scheduleTime = name => {
    const s = schedules[name]
    return `${String(s.hour).padStart(2, '0')}:${String(s.minute).padStart(2, '0')}`
  }

  const scheduleSummary = day => SCHEDULE_DAYS.find(d => d.value === day)?.summary || 'Daily'

  const nextRunText = name => {
    const s = schedules[name]
    if (!s.enabled) return 'Disabled'
    return `${scheduleSummary(s.day)} at ${String(s.hour).padStart(2, '0')}:${String(s.minute).padStart(2, '0')}`
  }

  const updateScheduleTime = (name, val) => {
    const [h = '00', m = '00'] = val.split(':')
    setSchedules(prev => ({
      ...prev,
      [name]: { ...prev[name], hour: parseInt(h) || 0, minute: parseInt(m) || 0 },
    }))
  }

  const handleSaveSchedule = async name => {
    if (isScheduleLocked(name)) return
    const s = schedules[name]
    try {
      await saveSchedule(name, s.enabled, s.day, s.hour, s.minute)
      setScheduleSaveStatus(prev => ({ ...prev, [name]: 'Saved.' }))
      setTimeout(() => setScheduleSaveStatus(prev => ({ ...prev, [name]: '' })), 2000)
    } catch {
      setScheduleSaveStatus(prev => ({ ...prev, [name]: 'Error saving.' }))
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
        <SectionLabel>Scheduled Playlists</SectionLabel>
        {PLAYLISTS.map(p => {
          const s = schedules[p.value]
          const locked = isScheduleLocked(p.value)
          return (
            <div key={p.value} className="py-2.5 border-b border-ui-border last:border-b-0">
              <div className="flex items-center gap-3.5">
                <label className={`flex items-center gap-2.5 ${locked ? 'opacity-45 cursor-not-allowed' : 'cursor-pointer'}`}>
                  <Toggle
                    checked={s.enabled}
                    onChange={v => {
                      setSchedules(prev => ({ ...prev, [p.value]: { ...prev[p.value], enabled: v } }))
                      setTimeout(() => handleSaveSchedule(p.value), 0)
                    }}
                    disabled={locked}
                  />
                  <span className="text-[14px] font-medium">{p.name}</span>
                </label>
                <button
                  disabled={locked}
                  onClick={() => setSchedules(prev => ({ ...prev, [p.value]: { ...prev[p.value], editing: !prev[p.value].editing } }))}
                  className="text-[12px] text-accent bg-surface border border-ui-border rounded-[6px] px-2.5 py-1 ml-auto cursor-pointer hover:border-accent hover:bg-[#242424] transition-colors disabled:opacity-45 disabled:cursor-not-allowed"
                >
                  {nextRunText(p.value)}
                </button>
                <span className="text-[12px] text-muted">{locked ? 'Set via Docker' : (scheduleSaveStatus[p.value] || '')}</span>
              </div>

              {s.editing && s.enabled && !locked && (
                <div className="flex flex-col gap-2 pt-2">
                  <div className="flex items-center gap-1.5">
                    <span className="text-[12px] text-muted">Runs</span>
                    <select
                      className={selectCls}
                      value={s.day}
                      onChange={e => setSchedules(prev => ({ ...prev, [p.value]: { ...prev[p.value], day: parseInt(e.target.value) } }))}
                    >
                      {SCHEDULE_DAYS.map(d => <option key={d.value} value={d.value}>{d.label}</option>)}
                    </select>
                    <span className="text-[12px] text-muted">at</span>
                    <input
                      type="time"
                      value={scheduleTime(p.value)}
                      onChange={e => updateScheduleTime(p.value, e.target.value)}
                      className="bg-surface border border-ui-border text-white rounded-[6px] px-2 py-1.5 text-[13px] outline-none focus:border-accent"
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button style={{ padding: '4px 12px', fontSize: 12 }}
                      onClick={() => { handleSaveSchedule(p.value); setSchedules(prev => ({ ...prev, [p.value]: { ...prev[p.value], editing: false } })) }}>
                      Save
                    </Button>
                    <Button style={{ padding: '4px 10px', fontSize: 12 }}
                      onClick={() => setSchedules(prev => ({ ...prev, [p.value]: { ...prev[p.value], editing: false } }))}>
                      ✕
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )
        })}
        <p className="text-[12px] text-muted mt-3">Schedule changes take effect after restarting the container.</p>
      </div>

      {/* Manual Run */}
      <div className="mt-6">
        <SectionLabel>Manual run</SectionLabel>
        <div className="flex gap-2.5 items-center flex-wrap mb-2.5">
          <label className="text-[12px] text-muted">Playlist</label>
          <select className={selectCls} value={playlist} onChange={e => setPlaylist(e.target.value)}>
            {PLAYLISTS.map(p => <option key={p.value} value={p.value}>{p.name}</option>)}
          </select>
          <label className="text-[12px] text-muted">Download mode</label>
          <select className={selectCls} value={dlmode} onChange={e => setDlmode(e.target.value)}>
            <option value="normal">normal</option>
            <option value="skip">skip</option>
            <option value="force">force</option>
          </select>
          <label className="flex items-center gap-1.5 text-[12px] text-muted cursor-pointer">
            <input type="checkbox" checked={noPersist} onChange={e => setNoPersist(e.target.checked)} /> no persist
          </label>
          <label className="flex items-center gap-1.5 text-[12px] text-muted cursor-pointer">
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

      {/* Recent Tracks */}
      {recentTracks.length > 0 && (
        <div className="mt-6">
          <SectionLabel>Recent tracks</SectionLabel>
          <Panel className="h-[400px]">
            {recentTracks.slice(0, 50).map((e, i) => (
              <div key={i} className="flex gap-2.5 items-baseline py-0.5">
                <span className="text-[11px] text-muted flex-shrink-0 tabular-nums">{e.time}</span>
                <span className="text-[12px] text-accent flex-shrink-0">{e.track}</span>
              </div>
            ))}
          </Panel>
        </div>
      )}

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

  const loadLog = () => {
    fetchLogs().then(text => {
      setLogFileEntries(text.split('\n').filter(l => l.trim()).map(l => ({ raw: l, ...parseSlogLine(l) })))
    })
  }

  useEffect(() => { loadLog() }, [])

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
        <Panel className="h-[400px]">
          {logFileEntries.map((e, i) => <LogRow key={i} entry={e} />)}
        </Panel>
      )}
    </div>
  )
}

// ── Settings ──────────────────────────────────────────────────────────────────
// Tab shell. Routes between Home, Settings, and Logs sections.

export default function Settings({ onWizard }) {
  const [activeTab, setActiveTab] = useState('run')

  return (
    <div className="bg-bg min-h-screen">
      <div className="max-w-[760px] mx-auto px-6 pb-12">
        <header className="flex items-center gap-4 pt-5 pb-0 border-b border-ui-border mb-6">
          <span className="text-[16px] font-bold tracking-tight">Explo</span>
          <nav className="flex gap-0.5 items-end">
            <button className={tabBtnCls(activeTab === 'run')} onClick={() => setActiveTab('run')}>Home</button>
            <button className={tabBtnCls(activeTab === 'config')} onClick={() => setActiveTab('config')}>Settings</button>
            <button className={tabBtnCls(activeTab === 'logs')} onClick={() => setActiveTab('logs')}>Logs</button>
          </nav>
        </header>

        {activeTab === 'run' && <HomeSection />}
        {activeTab === 'config' && <ConfigSection onWizard={onWizard} />}
        {activeTab === 'logs' && <LogsSection />}
      </div>
    </div>
  )
}
