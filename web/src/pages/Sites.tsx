import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Site } from '../lib/api'
import { Shell } from '../components/Shell'
import { Card, ErrorNote, Skeleton, Snippet } from '../components/ui'

function AddSiteForm() {
  const qc = useQueryClient()
  const [domain, setDomain] = useState('')
  const [error, setError] = useState('')

  const create = useMutation({
    mutationFn: (d: string) => api.post<Site>('/api/sites', { domain: d }),
    onSuccess: () => {
      setDomain('')
      setError('')
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
    onError: (e: Error) => setError(e.message),
  })

  return (
    <div className="mb-8">
      <form
        onSubmit={e => {
          e.preventDefault()
          create.mutate(domain.trim())
        }}
        className="flex flex-col gap-2 sm:flex-row"
      >
        <div className="flex-1">
          <label htmlFor="domain" className="sr-only">
            Domain
          </label>
          <input
            id="domain"
            name="domain"
            value={domain}
            onChange={e => setDomain(e.target.value)}
            placeholder="example.com"
            required
            autoComplete="off"
            spellCheck={false}
            aria-describedby="domain-hint"
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm placeholder:text-zinc-400 focus-visible:border-zinc-900 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-zinc-900"
          />
        </div>
        <button
          type="submit"
          disabled={create.isPending || !domain.trim()}
          className="rounded-lg bg-zinc-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-zinc-800 disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
        >
          {create.isPending ? 'Adding…' : 'Add site'}
        </button>
      </form>
      <p id="domain-hint" className="mt-2 text-xs text-zinc-500">
        Domain only — no scheme or path. A leading <code className="text-zinc-600">www.</code> is
        dropped, and subdomains report to the same site.
      </p>
      {error && <div className="mt-3"><ErrorNote message={error} /></div>}
    </div>
  )
}

function SiteRow({ site }: { site: Site }) {
  const [open, setOpen] = useState(false)

  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link
          to={`/sites/${site.id}`}
          className="text-base font-medium text-zinc-900 underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
        >
          {site.domain}
        </Link>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setOpen(o => !o)}
            aria-expanded={open}
            className="rounded-md px-2 py-1 text-sm text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
          >
            {open ? 'Hide snippet' : 'Snippet'}
          </button>
          <Link
            to={`/sites/${site.id}`}
            className="rounded-md bg-zinc-100 px-3 py-1 text-sm font-medium text-zinc-900 hover:bg-zinc-200 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
          >
            View stats
          </Link>
        </div>
      </div>
      {open && (
        <div className="mt-4">
          <Snippet domain={site.domain} />
        </div>
      )}
    </Card>
  )
}

export default function Sites() {
  const { data: sites, isPending, error } = useQuery({
    queryKey: ['sites'],
    queryFn: () => api.get<Site[]>('/api/sites'),
  })

  return (
    <Shell>
      <h1 className="mb-1 text-2xl font-semibold tracking-tight text-zinc-900">Your sites</h1>
      <p className="mb-6 text-sm text-zinc-500">
        Add a domain to get a snippet, then watch its traffic.
      </p>

      <AddSiteForm />

      {error && <ErrorNote message={(error as Error).message} />}

      {isPending ? (
        <div className="space-y-3">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : !sites?.length ? (
        <Card className="px-6 py-12 text-center">
          <h2 className="text-base font-medium text-zinc-900">No sites yet</h2>
          <p className="mx-auto mt-1 max-w-sm text-sm text-zinc-500">
            Add your first domain above. You&rsquo;ll get a one-line snippet to paste into its
            &lt;head&gt;, and traffic appears here within seconds.
          </p>
        </Card>
      ) : (
        <ul className="space-y-3">
          {sites.map(site => (
            <li key={site.id}>
              <SiteRow site={site} />
            </li>
          ))}
        </ul>
      )}
    </Shell>
  )
}
