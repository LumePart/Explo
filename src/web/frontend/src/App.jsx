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

  useEffect(() => {
    Promise.all([
      checkAuth(),
      fetchSetupStatus(),
      fetchBackgroundArt(),
    ]).then(([authed, status, artUrl]) => {
      if (artUrl) setBgUrl(artUrl)
      setIsFirstTime(status ? !status.wizard_complete : false)
      if (authed) {
        handleLoginSuccess({ fromLogin: false })
      } else {
        setView('login')
      }
    })
  }, [])

  async function handleLoginSuccess({ fromLogin = false } = {}) {
    const { values, sources } = await fetchConfig()
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
      onWizard={() => setView('wizard')}
      onLogout={() => logout().then(() => setView('login'))}
    />
  )
}
