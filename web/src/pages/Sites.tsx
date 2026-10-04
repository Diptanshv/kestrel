import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Site } from '../lib/api'
import { useAuth } from '../lib/auth-context'

function snippetFor(domain: string) {
  return `<script defer src="${window.location.origin}/script.js" data-domain="${domain}"></script>`
}

export default function Sites() {
  const { logout } = useAuth()
  const qc = useQueryClient()
  const [domain, setDomain] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState<number | null>(null)

  const { data: sites, isLoading } = useQuery({
    queryKey: ['sites'],
    queryFn: () => api.get<Site[]>('/api/sites'),
  })

  const create = useMutation({
    mutationFn: (d: string) => api.post<Site>('/api/sites', { domain: d }),
    onSuccess: () => { setDomain(''); setError(''); qc.invalidateQueries({ queryKey: ['sites'] }) },
    onError: (e: Error) => setError(e.message),
  })

  async function copy(site: Site) {
    await navigator.clipboard.writeText(snippetFor(site.domain))
    setCopied(site.id)
    setTimeout(() => setCopied(null), 1500)
  }

  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      <header className="mb-8 flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-zinc-900">Your sites</h1>
        <button onClick={() => logout()} className="text-sm text-zinc-600 underline">Sign out</button>
      </header>

      <form
        onSubmit={e => { e.preventDefault(); create.mutate(domain) }}
        className="mb-8 flex gap-2"
      >
        <input
          value={domain} onChange={e => setDomain(e.target.value)}
          placeholder="example.com" required
          className="flex-1 rounded-md border border-zinc-300 px-3 py-2 text-sm"
        />
        <button
          type="submit" disabled={create.isPending}
          className="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          Add site
        </button>
      </form>

      {error && <p className="mb-4 text-sm text-red-600">{error}</p>}

      {isLoading && <p className="text-sm text-zinc-500">Loading...</p>}

      {sites?.length === 0 && (
        <p className="text-sm text-zinc-500">No sites yet. Add one above to get a snippet.</p>
      )}

      <ul className="space-y-4">
        {sites?.map(site => (
          <li key={site.id} className="rounded-xl border border-zinc-200 bg-white p-4">
            <div className="flex items-center justify-between">
              <Link to={`/sites/${site.id}`} className="font-medium text-zinc-900 underline">
                {site.domain}
              </Link>
              <button onClick={() => copy(site)} className="text-sm text-zinc-600 underline">
                {copied === site.id ? 'Copied' : 'Copy snippet'}
              </button>
            </div>
            <pre className="mt-3 overflow-x-auto rounded-md bg-zinc-50 p-3 text-xs text-zinc-700">
              <code>{snippetFor(site.domain)}</code>
            </pre>
          </li>
        ))}
      </ul>
    </div>
  )
}