import { useState } from 'react'
import { login } from '../lib/api'

const inputCls = 'w-full bg-surface border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[15px] outline-none focus:border-accent disabled:opacity-45 disabled:cursor-not-allowed transition-colors'

export default function Login({ onSuccess }) {
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
    } catch (err) {
      setError('Invalid credentials')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="max-w-[420px] mx-auto">
      <div className="text-[20px] font-bold text-accent mb-6">Login</div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
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
          <div className="text-red-400 text-[13px]">{error}</div>
        )}

        <button
          type="submit"
          disabled={loading}
          className="bg-accent text-white rounded-[6px] px-6 py-2.5 text-[14px]"
        >
          {loading ? 'Logging in…' : 'Login'}
        </button>
      </form>
    </div>
  )
}
