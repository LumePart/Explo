/**
 * Wizard.jsx
 *
 * Three-step setup wizard. Owns all state and calls wizardStep1/2/3 to save
 * each step. Step components (Step1, Step2, Step3) are defined in this file —
 * they receive fields + setField from the Wizard component.
 *
 * Receives existing config/envSources from App to pre-populate fields.
 */

import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { wizardStep1, wizardStep2, wizardStep3, wizardStep4, testLidarr, fetchLidarrProfiles, fetchLidarrWebhookUrl } from '../lib/api'
import { ToggleRow } from './ui/Toggle'
import { DirInput } from './ui/DirInput'
import { TextField } from './ui/common'

const inputCls = 'w-full bg-surface border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[15px] outline-none focus:border-accent disabled:opacity-45 disabled:cursor-not-allowed transition-colors'

const NextBtn = ({ onClick, disabled, saving, label = 'Next →' }) => (
  <button
    onClick={onClick}
    disabled={disabled || saving}
    className="bg-accent text-white rounded-[6px] px-6 py-2.5 text-[14px] border-none cursor-pointer hover:opacity-85 disabled:opacity-40 disabled:cursor-not-allowed transition-opacity"
  >
    {saving ? 'Saving…' : label}
  </button>
)

const BackBtn = ({ onClick }) => (
  <button
    onClick={onClick}
    className="bg-transparent border border-ui-border text-muted rounded-[6px] px-5 py-2.5 text-[14px] cursor-pointer hover:border-white hover:text-white transition-colors mr-auto"
  >
    ← Back
  </button>
)

// ── Step 1: Discovery ─────────────────────────────────────────────────────────
// Collects the ListenBrainz username, discovery mode (playlist vs API), and
// which playlists the user wants to enable on a schedule.

const PLAYLISTS = [
  { value: 'weekly-exploration', name: 'Weekly Exploration', desc: '~50 tracks · refreshes every Tuesday' },
  { value: 'weekly-jams',        name: 'Weekly Jams',        desc: '~25 tracks · refreshes every Monday' },
  { value: 'daily-jams',         name: 'Daily Jams',         desc: '~25 tracks · refreshes daily' },
]

function Step1({ fields, setField, envSources, onNext, saving }) {
  const { user, discoveryMode, checked } = fields
  const isLocked = key => envSources[key] === 'env'
  const valid = user.trim() !== '' && (discoveryMode !== 'playlist' || Object.values(checked).some(Boolean))

  return (
    <div>
      <div className="text-[11px] text-muted uppercase tracking-[1px] mb-7">Step 1 of 4 — Discovery</div>
      <p className="text-[13px] text-muted mb-7 leading-relaxed">
        Explo uses your ListenBrainz listening history to find music recommendations.
      </p>

      <div className="flex flex-col gap-5">
        <TextField label="ListenBrainz username" labelFor="lb-user"
          hint={<>Don't have an account?{' '}<a href="https://listenbrainz.org" target="_blank" rel="noreferrer" className="text-accent">Sign up free.</a></>}>
          <input id="lb-user" type="text" className={inputCls} placeholder="e.g. musiclover42"
            autoComplete="off" spellCheck={false} value={user} onChange={e => setField('user', e.target.value)}
            disabled={isLocked('LISTENBRAINZ_USER')} />
        </TextField>

        <div className="flex flex-col gap-2">
          <label className="text-[13px] font-medium text-muted">Discovery mode</label>
          <div className="grid grid-cols-2 gap-2">
            {[
              { value: 'playlist', name: 'Playlist', desc: 'Pulls tracks from your ListenBrainz playlists on a schedule. Best once you have some listening history.' },
              { value: 'api',      name: 'API',      desc: '~25 tracks generated on demand. Use this if your ListenBrainz account is new or testing your setup.' },
            ].map(m => (
              <button
                key={m.value}
                onClick={() => setField('discoveryMode', m.value)}
                className={`text-left flex flex-col gap-[5px] px-4 py-3.5 bg-surface border rounded-[6px] cursor-pointer transition-colors
                  ${discoveryMode === m.value ? 'border-accent' : 'border-ui-border hover:border-[#404040]'}`}
              >
                <span className={`text-[13px] font-semibold ${discoveryMode === m.value ? 'text-accent' : 'text-white'}`}>{m.name}</span>
                <span className="text-[12px] text-muted leading-relaxed">{m.desc}</span>
              </button>
            ))}
          </div>
        </div>

        {discoveryMode === 'playlist' && (
          <div className="flex flex-col gap-2">
            <label className="text-[13px] font-medium text-muted">Which playlists should run on a schedule?</label>
            <div className="flex flex-col gap-0.5">
              {PLAYLISTS.map(p => (
                <ToggleRow
                  key={p.value}
                  checked={checked[p.value]}
                  onChange={v => setField('checked', { ...checked, [p.value]: v })}
                  name={p.name}
                  desc={p.desc}
                />
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="mt-8 flex justify-end">
        <NextBtn onClick={onNext} disabled={!valid} saving={saving} />
      </div>
    </div>
  )
}

// ── Step 2: Media System ──────────────────────────────────────────────────────
// Collects the media server type and its credentials. Fields shown/hidden
// conditionally based on which system is selected.

const SYSTEMS = [
  { value: 'jellyfin', name: 'Jellyfin' },
  { value: 'emby',     name: 'Emby' },
  { value: 'plex',     name: 'Plex' },
  { value: 'subsonic', name: 'Subsonic' },
  { value: 'mpd',      name: 'MPD' },
]

const API_KEY_SYSTEMS = ['jellyfin', 'emby', 'plex']

function Step2({ fields, setField, envSources, onBack, onNext, saving }) {
  const { system, systemUrl, apiKey, libraryName, systemUsername, systemPassword,
          playlistDir, sleepMinutes, publicPlaylist } = fields
  const isLocked = key => envSources[key] === 'env'

  const urlPlaceholder = () => {
    const ports = { jellyfin: '8096', emby: '8096', plex: '32400', subsonic: '4533' }
    return `e.g. http://192.168.1.100:${ports[system] || '8096'}`
  }

  const valid = () => {
    if (!system) return false
    if (system === 'mpd') return playlistDir.trim() !== ''
    if (!systemUrl) return false
    if (API_KEY_SYSTEMS.includes(system) && !apiKey) return false
    if (system === 'subsonic' && (!systemUsername || !systemPassword)) return false
    return true
  }

  return (
    <div>
      <div className="text-[11px] text-muted uppercase tracking-[1px] mb-7">Step 2 of 4 — Media System</div>
      <p className="text-[13px] text-muted mb-7 leading-relaxed">
        Explo will add discovered tracks to your library and create playlists automatically. It needs access to your media server to do this.
      </p>

      <div className="flex flex-col gap-5">
        <div className="flex flex-col gap-2">
          <label className="text-[13px] font-medium text-muted">Which media system do you use?</label>
          <div className="grid grid-cols-3 gap-2">
            {SYSTEMS.map(s => (
              <button
                key={s.value}
                onClick={() => setField('system', s.value)}
                className={`text-[14px] font-medium px-3 py-[18px] text-center bg-surface border rounded-[6px] cursor-pointer transition-colors
                  ${system === s.value ? 'border-accent text-accent' : 'border-ui-border text-white hover:border-[#404040]'}`}
              >
                {s.name}
              </button>
            ))}
          </div>
        </div>

        {system && system !== 'mpd' && (
          <TextField label="Server URL">
            <input type="text" className={inputCls} value={systemUrl} onChange={e => setField('systemUrl', e.target.value)}
              placeholder={urlPlaceholder()} disabled={isLocked('SYSTEM_URL')} />
          </TextField>
        )}

        {API_KEY_SYSTEMS.includes(system) && (
          <TextField label="API Key">
            <input type="text" className={inputCls} value={apiKey} onChange={e => setField('apiKey', e.target.value)}
              autoComplete="off" spellCheck={false} disabled={isLocked('API_KEY')} />
          </TextField>
        )}

        {API_KEY_SYSTEMS.includes(system) && (
          <TextField label="Library Name">
            <input type="text" className={inputCls} value={libraryName} onChange={e => setField('libraryName', e.target.value)}
              placeholder="e.g. Music" disabled={isLocked('LIBRARY_NAME')} />
          </TextField>
        )}

        {system === 'subsonic' && (
          <>
            <TextField label="Username">
              <input type="text" className={inputCls} value={systemUsername} onChange={e => setField('systemUsername', e.target.value)}
                autoComplete="off" disabled={isLocked('SYSTEM_USERNAME')} />
            </TextField>
            <TextField label="Password">
              <input type="password" className={inputCls} value={systemPassword} onChange={e => setField('systemPassword', e.target.value)}
                disabled={isLocked('SYSTEM_PASSWORD')} />
            </TextField>
          </>
        )}

        {system === 'mpd' && (
          <TextField label="Playlist directory" hint="Explo writes .m3u files here — MPD reads them as playlists.">
            <DirInput value={playlistDir} onChange={v => setField('playlistDir', v)} disabled={isLocked('PLAYLIST_DIR')}
              placeholder="e.g. /var/lib/mpd/playlists" />
          </TextField>
        )}

        {system && system !== 'mpd' && (
          <TextField label="Library scan wait"
            hint="Minutes Explo waits after triggering a library scan before creating playlists. Default: 2.">
            <input type="text" inputMode="numeric" className={inputCls} style={{ width: 80 }}
              value={sleepMinutes} onChange={e => setField('sleepMinutes', e.target.value)}
              placeholder="2" disabled={isLocked('SLEEP')} />
          </TextField>
        )}

        {system === 'subsonic' && (
          <ToggleRow
            checked={publicPlaylist}
            onChange={v => setField('publicPlaylist', v)}
            disabled={isLocked('PUBLIC_PLAYLIST')}
            name="Public playlists"
            desc="Make playlists visible to all users on the server"
          />
        )}
      </div>

      <div className="mt-8 flex">
        <BackBtn onClick={onBack} />
        <NextBtn onClick={onNext} disabled={!valid()} saving={saving} />
      </div>
    </div>
  )
}

// ── Step 3: Downloader ────────────────────────────────────────────────────────
// Collects download service selection (YouTube, Slskd) and their respective
// credentials, download directory, and file format preferences.

function Step3({ fields, setField, envSources, onBack, onNext, saving, isLastStep }) {
  const { downloadDir, useSubdirectory, migrateDownloads, dlServices,
          youtubeApiKey, trackExtension, filterList, slskdUrl, slskdApiKey } = fields
  const isLocked = key => envSources[key] === 'env'
  const showDownloadDir = dlServices.youtube || (dlServices.slskd && migrateDownloads)

  const valid = () => {
    if (!Object.values(dlServices).some(Boolean)) return false
    if (showDownloadDir && !downloadDir.trim()) return false
    if (dlServices.slskd && (!slskdUrl.trim() || !slskdApiKey.trim())) return false
    return true
  }

  return (
    <div>
      <div className="text-[11px] text-muted uppercase tracking-[1px] mb-7">Step 3 of 4 — Downloader</div>
      <p className="text-[13px] text-muted mb-7 leading-relaxed">
        Explo downloads tracks using one or both services. Enable what you have access to — if both are enabled, YouTube is tried first.
      </p>

      <div className="flex flex-col gap-4">
        <div>
          <ToggleRow
            checked={dlServices.youtube}
            onChange={v => setField('dlServices', { ...dlServices, youtube: v })}
            name="YouTube"
            desc="Downloads via yt-dlp · falls back to ytmusicapi if no API key is set"
          />
          {dlServices.youtube && (
            <div className="mt-3 pl-4 border-l-2 border-ui-border flex flex-col gap-4">
              <TextField label={<>YouTube API Key <span className="font-normal opacity-50">(optional)</span></>}
                hint={<>If set, uses the official YouTube Data API. Otherwise falls back to <strong>ytmusicapi</strong>.{' '}
                  <a href="https://console.cloud.google.com/apis/library/youtube.googleapis.com" target="_blank" rel="noreferrer" className="text-accent">Get an API key.</a></>}>
                <input type="text" className={inputCls} value={youtubeApiKey} onChange={e => setField('youtubeApiKey', e.target.value)}
                  autoComplete="off" spellCheck={false} placeholder="AIza…" disabled={isLocked('YOUTUBE_API_KEY')} />
              </TextField>
              <TextField label="Track format"
                hint={<>File format yt-dlp converts to. Default is <strong>opus</strong> — use <strong>mp3</strong> for broader device compatibility.</>}>
                <input type="text" className={inputCls} value={trackExtension} onChange={e => setField('trackExtension', e.target.value)}
                  placeholder="opus" autoComplete="off" spellCheck={false} disabled={isLocked('TRACK_EXTENSION')} />
              </TextField>
              <TextField label="Exclude keywords"
                hint="Comma-separated keywords to skip in YouTube results. Leave blank to use the defaults shown.">
                <input type="text" className={inputCls} value={filterList} onChange={e => setField('filterList', e.target.value)}
                  placeholder="live,remix,instrumental,extended,clean,acapella" autoComplete="off" spellCheck={false} disabled={isLocked('FILTER_LIST')} />
              </TextField>
              <TextField label="Download directory">
                <DirInput value={downloadDir} onChange={v => setField('downloadDir', v)} disabled={isLocked('DOWNLOAD_DIR')}
                  placeholder="e.g. /data/music/" />
              </TextField>
              <ToggleRow
                checked={useSubdirectory}
                onChange={v => setField('useSubdirectory', v)}
                disabled={isLocked('USE_SUBDIRECTORY')}
                name="Use playlist subfolders"
                desc="Create a subfolder per playlist inside the download directory"
              />
            </div>
          )}
        </div>

        <div>
          <ToggleRow
            checked={dlServices.slskd}
            onChange={v => setField('dlServices', { ...dlServices, slskd: v })}
            name="Slskd"
            desc="Downloads from the Soulseek P2P network · requires a running Slskd instance"
          />
          {dlServices.slskd && (
            <div className="mt-3 pl-4 border-l-2 border-ui-border flex flex-col gap-4">
              <TextField label="Slskd URL">
                <input type="text" className={inputCls} value={slskdUrl} onChange={e => setField('slskdUrl', e.target.value)}
                  placeholder="e.g. http://192.168.1.100:5030" disabled={isLocked('SLSKD_URL')} />
              </TextField>
              <TextField label="Slskd API Key">
                <input type="text" className={inputCls} value={slskdApiKey} onChange={e => setField('slskdApiKey', e.target.value)}
                  autoComplete="off" spellCheck={false} disabled={isLocked('SLSKD_API_KEY')} />
              </TextField>
              <p className="text-[12px] text-muted leading-relaxed">
                By default, tracks are saved to wherever Slskd is configured to download files.
              </p>
              <ToggleRow
                checked={migrateDownloads}
                onChange={v => setField('migrateDownloads', v)}
                disabled={isLocked('MIGRATE_DOWNLOADS')}
                name="Migrate downloads"
                desc="Move completed downloads to a separate directory instead"
              />
              {migrateDownloads && !dlServices.youtube && (
                <>
                  <TextField label="Download directory">
                    <DirInput value={downloadDir} onChange={v => setField('downloadDir', v)} disabled={isLocked('DOWNLOAD_DIR')}
                      placeholder="e.g. /data/music/" />
                  </TextField>
                  <ToggleRow
                    checked={useSubdirectory}
                    onChange={v => setField('useSubdirectory', v)}
                    disabled={isLocked('USE_SUBDIRECTORY')}
                    name="Use playlist subfolders"
                    desc="Create a subfolder per playlist inside the download directory"
                  />
                </>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="mt-8 flex">
        <BackBtn onClick={onBack} />
        <NextBtn onClick={onNext} disabled={!valid()} saving={saving} label={isLastStep ? 'Finish →' : 'Next →'} />
      </div>
    </div>
  )
}

// ── Step 4: Lidarr (optional) ─────────────────────────────────────────────────
// Optional integration that adds the Artist + Album to Lidarr when you rate a
// track in your Plex library. Skippable.

function Step4({ fields, setField, envSources, onBack, onFinish, saving }) {
  const { lidarrEnabled, lidarrUrl, lidarrApiKey, lidarrRootFolder,
          lidarrQualityProfileId, lidarrMetadataProfileId, lidarrPollInterval, lidarrWebhookEnabled } = fields
  const isLocked = key => envSources[key] === 'env'

  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null) // { ok, version|error }
  const [profiles, setProfiles] = useState(null) // { root_folders, quality_profiles, metadata_profiles }
  const [profilesError, setProfilesError] = useState('')
  const [webhookPath, setWebhookPath] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!lidarrEnabled) return
    fetchLidarrWebhookUrl().then(r => setWebhookPath(r.path)).catch(() => {})
  }, [lidarrEnabled])

  const handleTest = async () => {
    setTesting(true); setTestResult(null); setProfiles(null); setProfilesError('')
    try {
      const r = await testLidarr(lidarrUrl.trim(), lidarrApiKey.trim())
      setTestResult(r)
      if (r.ok) {
        try {
          const p = await fetchLidarrProfiles(lidarrUrl.trim(), lidarrApiKey.trim())
          setProfiles(p)
        } catch (e) {
          setProfilesError(e.message)
        }
      }
    } catch (e) {
      setTestResult({ ok: false, error: e.message })
    } finally {
      setTesting(false)
    }
  }

  const copyWebhook = async () => {
    if (!webhookPath) return
    const fullUrl = window.location.origin + webhookPath
    try {
      await navigator.clipboard.writeText(fullUrl)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }

  const valid = () => {
    if (!lidarrEnabled) return true
    if (!lidarrUrl.trim() || !lidarrApiKey.trim()) return false
    if (!lidarrRootFolder || !lidarrQualityProfileId || !lidarrMetadataProfileId) return false
    return true
  }

  return (
    <div>
      <div className="text-[11px] text-muted uppercase tracking-[1px] mb-7">Step 4 of 4 — Lidarr (optional)</div>
      <p className="text-[13px] text-muted mb-7 leading-relaxed">
        Connect Lidarr so that any track you rate in your Plex library gets its artist and album auto-added for permanent download. Skip this step if you don't use Lidarr.
      </p>

      <div className="flex flex-col gap-5">
        <ToggleRow
          checked={lidarrEnabled}
          onChange={v => setField('lidarrEnabled', v)}
          disabled={isLocked('LIDARR_ENABLED')}
          name="Enable Lidarr sync"
          desc="When on, rating any track in your Plex library triggers a Lidarr add"
        />

        {lidarrEnabled && (
          <>
            <TextField label="Lidarr URL">
              <input type="text" className={inputCls} value={lidarrUrl} onChange={e => setField('lidarrUrl', e.target.value)}
                placeholder="e.g. http://localhost:8686" disabled={isLocked('LIDARR_URL')} />
            </TextField>
            <TextField label="Lidarr API Key" hint="Settings → General in the Lidarr UI">
              <input type="password" className={inputCls} value={lidarrApiKey} onChange={e => setField('lidarrApiKey', e.target.value)}
                autoComplete="off" spellCheck={false} disabled={isLocked('LIDARR_API_KEY')} />
            </TextField>

            <div>
              <button
                onClick={handleTest}
                disabled={testing || !lidarrUrl.trim() || !lidarrApiKey.trim()}
                className="bg-transparent border border-ui-border text-white rounded-[6px] px-4 py-2 text-[13px] cursor-pointer hover:border-accent disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                {testing ? 'Testing…' : 'Test connection'}
              </button>
              {testResult && testResult.ok && (
                <span className="ml-3 text-[12px] text-accent">✓ Connected (Lidarr v{testResult.version})</span>
              )}
              {testResult && !testResult.ok && (
                <span className="ml-3 text-[12px] text-red-400">✗ {testResult.error || 'failed'}</span>
              )}
            </div>

            {profiles && (
              <>
                <TextField label="Root folder">
                  <select className={inputCls} value={lidarrRootFolder} onChange={e => setField('lidarrRootFolder', e.target.value)}>
                    <option value="">— select —</option>
                    {profiles.root_folders.map(f => (
                      <option key={f.id} value={f.path}>{f.path}</option>
                    ))}
                  </select>
                </TextField>
                <TextField label="Quality profile">
                  <select className={inputCls} value={lidarrQualityProfileId} onChange={e => setField('lidarrQualityProfileId', e.target.value)}>
                    <option value="">— select —</option>
                    {profiles.quality_profiles.map(p => (
                      <option key={p.id} value={p.id}>{p.name}</option>
                    ))}
                  </select>
                </TextField>
                <TextField label="Metadata profile">
                  <select className={inputCls} value={lidarrMetadataProfileId} onChange={e => setField('lidarrMetadataProfileId', e.target.value)}>
                    <option value="">— select —</option>
                    {profiles.metadata_profiles.map(p => (
                      <option key={p.id} value={p.id}>{p.name}</option>
                    ))}
                  </select>
                </TextField>
              </>
            )}
            {profilesError && (
              <div className="text-[12px] text-red-400">Profiles load failed: {profilesError}</div>
            )}

            <TextField label="Polling interval"
              hint="Webhook fallback poll cadence — Go duration string (e.g. 5m, 15m, 1h).">
              <input type="text" className={inputCls} style={{ width: 120 }}
                value={lidarrPollInterval} onChange={e => setField('lidarrPollInterval', e.target.value)}
                placeholder="15m" disabled={isLocked('LIDARR_POLL_INTERVAL')} />
            </TextField>

            <ToggleRow
              checked={lidarrWebhookEnabled}
              onChange={v => setField('lidarrWebhookEnabled', v)}
              disabled={isLocked('LIDARR_WEBHOOK_ENABLED')}
              name="Plex webhook listener"
              desc="Real-time rating events (requires Plex Pass)"
            />

            {lidarrWebhookEnabled && webhookPath && (
              <TextField label="Plex webhook URL"
                hint="Paste this into Plex → Settings → Webhooks. The token is generated per-instance and is required.">
                <div className="flex gap-2">
                  <input type="text" readOnly className={inputCls}
                    value={typeof window !== 'undefined' ? window.location.origin + webhookPath : webhookPath} />
                  <button onClick={copyWebhook}
                    className="bg-transparent border border-ui-border text-white rounded-[6px] px-3 py-2 text-[13px] cursor-pointer hover:border-accent transition-colors whitespace-nowrap">
                    {copied ? 'Copied!' : 'Copy'}
                  </button>
                </div>
              </TextField>
            )}
          </>
        )}
      </div>

      <div className="mt-8 flex">
        <BackBtn onClick={onBack} />
        <NextBtn onClick={onFinish} disabled={!valid()} saving={saving} label="Finish →" />
      </div>
    </div>
  )
}

// ── Wizard ────────────────────────────────────────────────────────────────────
// Owns all wizard state and calls wizardStep1/2/3 APIs to save each step.
// Receives existing config/envSources from App to pre-populate fields.

const SPRING = { type: 'spring', stiffness: 280, damping: 28, mass: 0.9, opacity: { duration: 0.2, ease: 'easeInOut' } }
const SLIDE_VARIANTS = {
  enter: dir => ({ opacity: 0, x: dir * 180, scale: 0.96 }),
  center: { opacity: 1, x: 0 },
  exit: dir => ({ opacity: 0, x: dir * -180, scale: 0.96 }),
}
const FADE_VARIANTS = {
  enter: { opacity: 0 },
  center: { opacity: 1 },
  exit: { opacity: 0 },
}

export default function Wizard({ config, envSources, onComplete }) {
  const [step, setStep] = useState(1)
  const [stepDirection, setStepDirection] = useState(1)
  const [saving, setSaving] = useState(false)
  const reduceMotion = useReducedMotion()
  const variants = reduceMotion ? FADE_VARIANTS : SLIDE_VARIANTS
  const transition = reduceMotion ? { duration: 0.18 } : SPRING

  const [fields, setFields] = useState(() => {
    const s = (config.DOWNLOAD_SERVICES || '').split(',')
    return {
      // Step 1
      user:             config.LISTENBRAINZ_USER || '',
      discoveryMode:    config.LISTENBRAINZ_DISCOVERY || 'playlist',
      checked: {
        'weekly-exploration': !!config.WEEKLY_EXPLORATION_SCHEDULE,
        'weekly-jams':        !!config.WEEKLY_JAMS_SCHEDULE,
        'daily-jams':         !!config.DAILY_JAMS_SCHEDULE,
      },
      // Step 2
      system:           config.EXPLO_SYSTEM || '',
      systemUrl:        config.SYSTEM_URL || '',
      apiKey:           config.API_KEY || '',
      libraryName:      config.LIBRARY_NAME || '',
      systemUsername:   config.SYSTEM_USERNAME || '',
      systemPassword:   config.SYSTEM_PASSWORD || '',
      playlistDir:      config.PLAYLIST_DIR || '',
      sleepMinutes:     config.SLEEP || '',
      publicPlaylist:   config.PUBLIC_PLAYLIST === 'true',
      // Step 3
      downloadDir:      config.DOWNLOAD_DIR || '',
      useSubdirectory:  config.USE_SUBDIRECTORY !== 'false',
      migrateDownloads: config.MIGRATE_DOWNLOADS === 'true',
      dlServices:       { youtube: s.includes('youtube'), slskd: s.includes('slskd') },
      youtubeApiKey:    config.YOUTUBE_API_KEY || '',
      trackExtension:   config.TRACK_EXTENSION || '',
      filterList:       config.FILTER_LIST || '',
      slskdUrl:         config.SLSKD_URL || '',
      slskdApiKey:      config.SLSKD_API_KEY || '',
      // Step 4
      lidarrEnabled:           config.LIDARR_ENABLED === 'true',
      lidarrUrl:               config.LIDARR_URL || '',
      lidarrApiKey:            config.LIDARR_API_KEY || '',
      lidarrRootFolder:        config.LIDARR_ROOT_FOLDER || '',
      lidarrQualityProfileId:  config.LIDARR_QUALITY_PROFILE_ID || '',
      lidarrMetadataProfileId: config.LIDARR_METADATA_PROFILE_ID || '',
      lidarrPollInterval:      config.LIDARR_POLL_INTERVAL || '15m',
      lidarrWebhookEnabled:    config.LIDARR_WEBHOOK_ENABLED !== 'false',
    }
  })

  const setField = (key, val) => setFields(prev => ({ ...prev, [key]: val }))
  const goToStep = nextStep => {
    setStepDirection(nextStep > step ? 1 : -1)
    setStep(nextStep)
  }

  const lockedKeys = Object.entries(envSources)
    .filter(([k, s]) => s === 'env' && !k.endsWith('_SCHEDULE') && !k.endsWith('_FLAGS'))
    .map(([k]) => k)

  async function handleStep1() {
    setSaving(true)
    try {
      const playlists = Object.entries(fields.checked).filter(([, v]) => v).map(([k]) => k)
      await wizardStep1(fields.user.trim(), playlists, fields.discoveryMode)
      goToStep(2)
    } catch (e) {
      alert('Error saving: ' + e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleStep2() {
    setSaving(true)
    try {
      await wizardStep2({
        system: fields.system, url: fields.systemUrl, api_key: fields.apiKey,
        library_name: fields.libraryName, username: fields.systemUsername,
        password: fields.systemPassword, playlist_dir: fields.playlistDir,
        sleep: fields.sleepMinutes, public_playlist: fields.publicPlaylist,
      })
      goToStep(3)
    } catch (e) {
      alert('Error saving: ' + e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleStep3() {
    setSaving(true)
    try {
      const services = Object.entries(fields.dlServices).filter(([, v]) => v).map(([k]) => k)
      await wizardStep3({
        download_dir: fields.downloadDir, use_subdirectory: fields.useSubdirectory,
        migrate_downloads: fields.migrateDownloads, download_services: services,
        youtube_api_key: fields.youtubeApiKey, track_extension: fields.trackExtension,
        filter_list: fields.filterList, slskd_url: fields.slskdUrl, slskd_api_key: fields.slskdApiKey,
      })
      if (fields.system === 'plex') {
        setStep(4)
      } else {
        onComplete()
      }
    } catch (e) {
      alert('Error saving: ' + e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleStep4() {
    setSaving(true)
    try {
      await wizardStep4({
        enabled:             fields.lidarrEnabled,
        url:                 fields.lidarrUrl.trim(),
        api_key:             fields.lidarrApiKey.trim(),
        root_folder:         fields.lidarrRootFolder,
        quality_profile_id:  Number(fields.lidarrQualityProfileId) || 0,
        metadata_profile_id: Number(fields.lidarrMetadataProfileId) || 0,
        poll_interval:       fields.lidarrPollInterval || '15m',
        webhook_enabled:     fields.lidarrWebhookEnabled,
      })
      onComplete()
    } catch (e) {
      alert('Error saving: ' + e.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="min-h-screen bg-bg flex items-start pt-[15vh] overflow-y-auto">
      <div className="fixed top-6 left-7 text-[20px] font-bold tracking-tight text-accent">Explo</div>

      <div className="max-w-[520px] w-full mx-auto px-6 py-12 overflow-hidden">
        {lockedKeys.length > 0 && (
          <div className="text-[12px] text-muted bg-surface border border-ui-border rounded-[6px] px-3.5 py-2.5 mb-6 leading-relaxed">
            You've set the following in your Docker environment, so they can't be changed here:{' '}
            <strong>{lockedKeys.join(', ')}</strong>
          </div>
        )}

        <AnimatePresence initial={false} mode="popLayout" custom={stepDirection}>
          <motion.div
            key={step}
            custom={stepDirection}
            variants={variants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={transition}
          >
              {step === 1 && (
                <Step1
                  fields={fields} setField={setField}
                  envSources={envSources}
                  onNext={handleStep1} saving={saving}
                />
              )}
              {step === 2 && (
                <Step2
                  fields={fields} setField={setField}
                  envSources={envSources}
                  onBack={() => goToStep(1)} onNext={handleStep2} saving={saving}
                />
              )}
              {step === 3 && (
                <Step3
                  fields={fields} setField={setField}
                  envSources={envSources}
                  onBack={() => goToStep(2)} onNext={handleStep3} saving={saving}
                  isLastStep={fields.system !== 'plex'}
                />
              )}
              {step === 4 && (
                <Step4
                  fields={fields} setField={setField}
                  envSources={envSources}
                  onBack={() => setStep(3)} onFinish={handleStep4} saving={saving}
                />
              )}
            </motion.div>
        </AnimatePresence>
      </div>
    </div>
  )
}
