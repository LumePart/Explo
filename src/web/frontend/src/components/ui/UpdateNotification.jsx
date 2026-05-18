// Polls GitHub releases API and compares to build version, shows a modal if a new version is out
import { useState, useEffect } from 'react'
import { marked } from 'marked'

const GITHUB_RELEASE_URL = 'https://api.github.com/repos/LumePart/Explo/releases/latest'
const SEEN_VERSION_KEY = 'explo_seen_version'
const RUNNING_VERSION = import.meta.env.VITE_VERSION || 'dev'

function parseVersion(v) {
  return (v || '').replace(/^v/, '').split('.').map(Number)
}

function isNewer(latest, current) {
  const l = parseVersion(latest)
  const c = parseVersion(current)
  for (let i = 0; i < 3; i++) {
    if (l[i] > c[i]) return true
    if (l[i] < c[i]) return false
  }
  return false
}

function ArrowUpCircleIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" />
      <polyline points="16 12 12 8 8 12" />
      <line x1="12" y1="16" x2="12" y2="8" />
    </svg>
  )
}

export function UpdateNotification() {
  const [updateInfo, setUpdateInfo] = useState(null)
  const [open, setOpen] = useState(false)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    if (RUNNING_VERSION === 'dev') return

    function check() {
      fetch(GITHUB_RELEASE_URL, { headers: { Accept: 'application/vnd.github+json' } })
        .then(r => r.ok ? r.json() : null)
        .then(release => {
          if (!release?.tag_name) return
          if (!isNewer(release.tag_name, RUNNING_VERSION)) return

          setUpdateInfo({
            latestVersion: release.tag_name,
            releaseName: release.name,
            releaseNotes: release.body,
            releaseUrl: release.html_url,
          })

          const seen = localStorage.getItem(SEEN_VERSION_KEY)
          if (seen === release.tag_name) setDismissed(true)
          else setOpen(true)
        })
        .catch(() => {})
    }

    check()
    const interval = setInterval(check, 12 * 60 * 60 * 1000)
    return () => clearInterval(interval)
  }, [])

  function handleDismiss() {
    localStorage.setItem(SEEN_VERSION_KEY, updateInfo.latestVersion)
    setDismissed(true)
    setOpen(false)
  }

  if (!updateInfo) return null

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title={`Update available: ${updateInfo.latestVersion}`}
        className="relative pb-2 cursor-pointer bg-transparent border-none flex items-end"
        style={{ color: dismissed ? 'var(--color-muted)' : '#f59e0b' }}
      >
        <ArrowUpCircleIcon />
        {!dismissed && (
          <span
            className="absolute top-1 -right-0.5 w-1.5 h-1.5 rounded-full"
            style={{ background: '#f59e0b' }}
          />
        )}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: 'rgba(0,0,0,0.6)' }}
          onClick={e => { if (e.target === e.currentTarget) handleDismiss() }}
        >
          <div
            className="bg-surface border border-ui-border rounded-[8px] w-full max-w-lg mx-4 flex flex-col"
            style={{ maxHeight: '80vh' }}
          >
            <div className="flex items-start justify-between p-5 border-b border-ui-border gap-4">
              <div>
                <div className="text-[11px] uppercase tracking-[1px] text-muted mb-1">Update available</div>
                <div className="text-[20px] font-bold text-white leading-none">
                  {updateInfo.latestVersion}
                </div>
                {updateInfo.releaseName && updateInfo.releaseName !== updateInfo.latestVersion && (
                  <div className="text-[13px] text-muted mt-1">{updateInfo.releaseName}</div>
                )}
              </div>
              <div className="text-[11px] text-muted text-right shrink-0 pt-1">
                Current: {RUNNING_VERSION}
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
