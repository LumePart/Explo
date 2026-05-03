import { useState, useEffect } from 'react'
import { checkAuth, fetchConfig } from './lib/api'
import Login from './components/Login'
import Wizard from './components/Wizard'
import Settings from './components/Settings'

export default function App() {
  const [view, setView] = useState(null)
  const [config, setConfig] = useState({})
  const [envSources, setEnvSources] = useState({})

  useEffect(() => {
    checkAuth().then(authed => {
      if (authed) {
        handleLoginSuccess()
      } else {
        setView('login')
      }
    })
  }, [])

  async function handleLoginSuccess() {
    const { values, sources } = await fetchConfig()
    setConfig(values)
    setEnvSources(sources || {})
    setView(values.WIZARD_COMPLETE === 'true' ? 'settings' : 'wizard')
  }

  if (view === null) return <div className="min-h-screen bg-bg" />

  if (view === 'login') {
    return (
      <div className="min-h-screen bg-bg flex items-center">
        <div className="max-w-[520px] w-full mx-auto px-6 py-12">
          <div className="text-[20px] font-bold tracking-tight text-accent mb-10">Explo</div>
          <Login onSuccess={handleLoginSuccess} />
        </div>
      </div>
    )
  }

  if (view === 'wizard') {
    return (
      <Wizard
        config={config}
        envSources={envSources}
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

  return <Settings onWizard={() => setView('wizard')} />
}
