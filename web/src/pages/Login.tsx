import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../lib/auth-context'
import { ErrorNote } from '../components/ui'

export default function Login() {
  const { user, loading, login, register } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (loading) return null
  if (user) return <Navigate to="/sites" replace />

  const registering = mode === 'register'

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await (registering ? register(email, password) : login(email, password))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  const field =
    'w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm placeholder:text-zinc-400 focus-visible:border-zinc-900 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-zinc-900'

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-zinc-50 px-4 py-12">
      <div className="mb-6 flex items-center gap-2">
        <span aria-hidden className="h-6 w-6 rounded-md bg-zinc-900" />
        <span className="text-lg font-semibold tracking-tight text-zinc-900">kestrel</span>
      </div>

      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm space-y-4 rounded-xl border border-zinc-200 bg-white p-6 shadow-sm"
      >
        <div>
          <h1 className="text-lg font-semibold text-zinc-900">
            {registering ? 'Create an account' : 'Sign in'}
          </h1>
          <p className="mt-1 text-sm text-zinc-500">
            Privacy-first analytics. No cookies, no consent banner.
          </p>
        </div>

        <div className="space-y-1">
          <label htmlFor="email" className="block text-sm font-medium text-zinc-700">
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            required
            autoFocus
            autoComplete="email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            className={field}
          />
        </div>

        <div className="space-y-1">
          <label htmlFor="password" className="block text-sm font-medium text-zinc-700">
            Password
          </label>
          <input
            id="password"
            name="password"
            type="password"
            required
            minLength={8}
            autoComplete={registering ? 'new-password' : 'current-password'}
            aria-describedby={registering ? 'password-hint' : undefined}
            value={password}
            onChange={e => setPassword(e.target.value)}
            className={field}
          />
          {registering && (
            <p id="password-hint" className="text-xs text-zinc-500">
              At least 8 characters.
            </p>
          )}
        </div>

        {error && <ErrorNote message={error} />}

        <button
          type="submit"
          disabled={busy}
          className="w-full rounded-lg bg-zinc-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-zinc-800 disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
        >
          {busy ? 'Working…' : registering ? 'Create account' : 'Sign in'}
        </button>

        <p className="text-center text-sm text-zinc-500">
          {registering ? 'Already have an account?' : 'Need an account?'}{' '}
          <button
            type="button"
            onClick={() => {
              setMode(registering ? 'login' : 'register')
              setError('')
            }}
            className="rounded font-medium text-zinc-900 underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
          >
            {registering ? 'Sign in' : 'Create one'}
          </button>
        </p>
      </form>
    </div>
  )
}
