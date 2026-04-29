import { useState, useEffect } from 'react'
import { fetchConfig } from './lib/api'
import Wizard from './components/Wizard'
import Settings from './components/Settings'

export default function App() {
  const [view, setView] = useState(null)
  const [config, setConfig] = useState({})
  const [envSources, setEnvSources] = useState({})

  useEffect(() => {
    fetchConfig().then(({ values, sources }) => {
      setConfig(values)
      setEnvSources(sources || {})
      setView(values.WIZARD_COMPLETE === 'true' ? 'settings' : 'wizard')
    })
  }, [])

  if (!view) return <div className="min-h-screen bg-bg" />

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
