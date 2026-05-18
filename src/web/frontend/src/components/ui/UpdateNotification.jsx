// Polls GitHub releases API and compares to build version, shows a modal if a new version is out
import { useState, useEffect } from 'react'
import { marked } from 'marked'

const GITHUB_RELEASE_URL = 'https://api.github.com/repos/LumePart/Explo/releases/latest'
const SEEN_VERSION_KEY = 'explo_seen_version'
const CACHE_KEY = 'explo_update_cache'
const CHECK_INTERVAL = 12 * 60 * 60 * 1000
const RUNNING_VERSION = import.meta.env.VITE_VERSION || 'dev'

function parseVersion(v) {
  return (v || '').replace(/^v/, '').split('.').map(Number)
}

function isNewer(latest, current) {
  const l = parseVersion(latest)
  const c = parseVersion(current)
  for (let i = 0; i < 3; i++) {
    const lv = l[i] || 0
    const cv = c[i] || 0
    if (lv > cv) return true
    if (lv < cv) return false
  }
  return false
}

export function UpdateNotification() {
  const [updateInfo, setUpdateInfo] = useState(null)
  const [open, setOpen] = useState(false)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    if (RUNNING_VERSION === 'dev') return

    // Restore cached result immediately for UI, fetch in background if needed
    try {
      const cached = JSON.parse(localStorage.getItem(CACHE_KEY))
      if (cached?.latestVersion && isNewer(cached.latestVersion, RUNNING_VERSION)) {
        setUpdateInfo(cached)
        const seen = localStorage.getItem(SEEN_VERSION_KEY)
        if (seen === cached.latestVersion) setDismissed(true)
        else setOpen(true)
      }
    } catch {}

    function fetchUpdate() {
      fetch(GITHUB_RELEASE_URL, { headers: { Accept: 'application/vnd.github+json' } })
        .then(r => r.ok ? r.json() : null)
        .then(release => {
          localStorage.setItem(CACHE_KEY + '_ts', String(Date.now()))
          if (!release?.tag_name) return
          if (!isNewer(release.tag_name, RUNNING_VERSION)) return

          const info = {
            latestVersion: release.tag_name,
            releaseName: release.name,
            releaseNotes: release.body,
            releaseUrl: release.html_url,
          }
          localStorage.setItem(CACHE_KEY, JSON.stringify(info))
          setUpdateInfo(info)

          const seen = localStorage.getItem(SEEN_VERSION_KEY)
          if (seen === release.tag_name) setDismissed(true)
          else setOpen(true)
        })
        .catch(() => {})
    }

    // Only fetch if 12 hours have passed since last check
    const lastCheck = Number(localStorage.getItem(CACHE_KEY + '_ts')) || 0
    if (Date.now() - lastCheck >= CHECK_INTERVAL) fetchUpdate()

    const interval = setInterval(() => {
      const last = Number(localStorage.getItem(CACHE_KEY + '_ts')) || 0
      if (Date.now() - last >= CHECK_INTERVAL) fetchUpdate()
    }, CHECK_INTERVAL)
    return () => clearInterval(interval)
  }, [])

  function handleDismiss() {
    localStorage.setItem(SEEN_VERSION_KEY, updateInfo.latestVersion)
    setDismissed(true)
    setOpen(false)
  }

  const hasUpdate = !!updateInfo
  const isNew = hasUpdate && !dismissed

  return (
    <>
      {/* Bottom-left version indicator */}
      <button
        onClick={hasUpdate ? () => setOpen(true) : undefined}
        className="fixed bottom-4 left-4 z-40 bg-transparent border-none text-[11px] transition-colors"
        style={{
          color: hasUpdate ? '#8e8058' : '#4a4a4a',
          cursor: hasUpdate ? 'pointer' : 'default',
        }}
        onMouseEnter={e => { if (hasUpdate) e.currentTarget.style.color = '#f5c842' }}
        onMouseLeave={e => { if (hasUpdate) e.currentTarget.style.color = '#8e8058' }}
      >
        {hasUpdate
          ? `Explo ${RUNNING_VERSION} — update available (${updateInfo.latestVersion})`
          : `Explo ${RUNNING_VERSION}`}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: '#00000066' }}
          onClick={e => { if (e.target === e.currentTarget) handleDismiss() }}
        >
          <div
            className="border border-ui-border rounded-[8px] w-full max-w-lg mx-4 flex flex-col backdrop-blur-xl animate-fade-up"
            style={{ maxHeight: '80vh', background: '#1a221c99' }}
          >
            <div className="flex items-start justify-between p-5 border-b border-ui-border gap-4">
              <div>
                <div className="text-[28px] uppercase mb-1 text-white" style={{ fontFamily: "'Bebas Neue', sans-serif", letterSpacing: '0.025em', lineHeight: 0.95 }}>Update available</div>
                <div className="text-[13px] font-bold text-white leading-none">
                  {updateInfo.latestVersion}
                </div>
                {updateInfo.releaseName && updateInfo.releaseName !== updateInfo.latestVersion && (
                  <div className="text-[13px] text-muted mt-1">{updateInfo.releaseName}</div>
                )}
              </div>
            </div>

            {updateInfo.releaseNotes && (
              <div
                className="markdown overflow-y-auto p-5 flex-1"
                dangerouslySetInnerHTML={{ __html: marked.parse(updateInfo.releaseNotes) }}
              />
            )}

            <div className="flex items-center justify-end gap-2 p-4 border-t border-ui-border">
              <button
                onClick={handleDismiss}
                className="bg-transparent border-none text-[12px] text-muted hover:text-white transition-colors cursor-pointer px-3 py-1.5"
              >
                Dismiss
              </button>
              <a
                href={updateInfo.releaseUrl}
                target="_blank"
                rel="noreferrer"
                onClick={handleDismiss}
                className="bg-surface border border-ui-border text-white rounded-[6px] px-4 py-1.5 text-[12px] cursor-pointer hover:border-accent transition-colors no-underline"
              >
                View on GitHub
              </a>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
