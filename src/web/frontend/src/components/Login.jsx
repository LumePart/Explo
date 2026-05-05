import { useState } from 'react'
import { login } from '../lib/api'

const inputCls = 'w-full bg-surface border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[15px] outline-none focus:border-accent disabled:opacity-45 disabled:cursor-not-allowed transition-colors'

export default function Login({ isFirstTime, bgUrl, bgLoaded, onBgLoad, onSuccess }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e) {
    e.preventDefault()
    setLoading(true)
    setError('')

    try {
      await login(username, password)
      onSuccess()
    } catch {
      setError('Invalid credentials')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen relative overflow-hidden bg-bg flex items-center">

      {/* Artwork backdrop — fade in only after image data is loaded */}
      {bgUrl && (
        <img
          src={bgUrl}
          onLoad={onBgLoad}
          className={`absolute inset-0 z-0 w-full h-full object-cover transition-opacity duration-[1500ms] ${bgLoaded ? 'opacity-100' : 'opacity-0'}`}
          style={{ filter: 'brightness(0.45) saturate(1.2)' }}
          alt=""
        />
      )}

      {/* Dark base gradient — vertical on mobile, horizontal on desktop */}
      <div className="absolute inset-0 z-0 bg-gradient-to-b from-bg/40 via-bg/60 to-bg/85 sm:bg-gradient-to-r sm:from-bg sm:via-bg/80 sm:to-bg/20" />
      {/* Green accent tint */}
      <div className="absolute inset-0 z-0 bg-gradient-to-b from-transparent to-accent/8 sm:bg-gradient-to-r sm:from-accent/20 sm:via-accent/8 sm:to-transparent" />

      {/* Form — centered on mobile, left-anchored on desktop */}
      <div className="relative z-10 w-full px-6 sm:px-0 sm:ml-[8%] sm:max-w-[400px]">

        <div className="text-[11px] tracking-[3px] uppercase text-muted mb-8 font-semibold">
          explo
        </div>

        <h1 className="text-[32px] sm:text-[40px] font-bold leading-tight text-white mb-1">
          {isFirstTime ? "Let's get started" : 'Welcome back'}<span className="text-accent">.</span>
        </h1>
        <p className="text-muted text-[14px] mb-7">
          {isFirstTime ? 'Sign in to begin setup.' : 'Sign in to continue.'}
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <input
            className={inputCls}
            type="text"
            placeholder="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />

          <input
            className={inputCls}
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />

          {error && (
            <div className="text-danger text-[13px]">{error}</div>
          )}

          <div className="pt-2">
            <button
              type="submit"
              disabled={loading}
              className="rounded-full bg-accent text-black px-8 py-3 text-[13px] font-bold tracking-[2px] uppercase disabled:opacity-45 disabled:cursor-not-allowed transition-opacity"
            >
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
