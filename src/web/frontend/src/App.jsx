import { useState, useEffect } from 'react'
import { checkAuth, fetchConfig, fetchSetupStatus, fetchBackgroundArt, logout } from './lib/api'
import Login from './components/Login'
import Wizard from './components/Wizard'
import Settings from './components/Settings'

export default function App() {
  const [view, setView] = useState(null)
  const [config, setConfig] = useState({})
  const [envSources, setEnvSources] = useState({})
  const [isFirstTime, setIsFirstTime] = useState(false)
  const [bgUrl, setBgUrl] = useState(null)
  const [bgLoaded, setBgLoaded] = useState(false)
  const [fadingOut, setFadingOut] = useState(false)
  const [authDisabled, setAuthDisabled] = useState(false)
  const [loadError, setLoadError] = useState(null)

  useEffect(() => {
    Promise.all([
      checkAuth(),
      fetchSetupStatus(),
    ]).then(([{ authenticated, authDisabled }, status]) => {
      setIsFirstTime(status ? !status.wizard_complete : false)
      setAuthDisabled(authDisabled)
      if (authenticated) {
        handleLoginSuccess({ fromLogin: false })
      } else {
        setView('login')
      }
    }).catch((e) => {
      // A server hiccup during startup used to leave the page blank
      // forever: the view never left its initial null state.
      setLoadError(e?.message || 'failed to reach the server')
      setView('error')
    })
    fetchBackgroundArt().then((artUrl) => {
      if (artUrl) setBgUrl(artUrl)
    })
  }, [])

  async function handleLoginSuccess({ fromLogin = false } = {}) {
    let values, sources
    try {
      ({ values, sources } = await fetchConfig())
    } catch (e) {
      setLoadError(e?.message || 'failed to load configuration')
      setView('error')
      return
    }
    setConfig(values)
    setEnvSources(sources || {})
    const nextView = values.WIZARD_COMPLETE === 'true' ? 'settings' : 'wizard'
    if (nextView === 'wizard' && fromLogin) {
      setFadingOut(true)
      setTimeout(() => {
        setView('wizard')
        setFadingOut(false)
      }, 350)
    } else {
      setView(nextView)
    }
  }

  if (view === 'error') {
    return (
      <div className="min-h-screen bg-bg flex items-center justify-center p-6">
        <div className="max-w-md text-center">
          <h1 className="text-xl font-semibold mb-2">Couldn&apos;t load Explo</h1>
          <p className="text-sm text-muted mb-4">{loadError}</p>
          <button
            className="px-4 py-2 rounded bg-accent text-white"
            onClick={() => window.location.reload()}
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  if (view === null) return <div className="min-h-screen bg-bg" />

  if (view === 'login') {
    return (
      <div className={`transition-all duration-300 ${fadingOut ? 'opacity-0 blur-sm' : ''}`}>
        <Login
          isFirstTime={isFirstTime}
          bgUrl={bgUrl}
          bgLoaded={bgLoaded}
          onBgLoad={() => setBgLoaded(true)}
          onSuccess={() => handleLoginSuccess({ fromLogin: true })}
        />
      </div>
    )
  }

  if (view === 'wizard') {
    return (
      <Wizard
        config={config}
        envSources={envSources}
        bgUrl={bgUrl}
        bgLoaded={bgLoaded}
        onBgLoad={() => setBgLoaded(true)}
        onComplete={() => {
          fetchConfig().then(({ values, sources }) => {
            setConfig(values)
            setEnvSources(sources || {})
            setView('settings')
          })
        }}
      />
    )
  }

  return (
    <Settings
      authDisabled={authDisabled}
      onWizard={() => setView('wizard')}
      onLogout={() => logout().then(() => setView('login'))}
    />
  )
}
