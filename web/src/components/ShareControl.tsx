import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Site } from '../lib/api'
import { ErrorNote } from './ui'

/** Owner-only toggle for the read-only public dashboard and its link. */
export function ShareControl({ site }: { site: Site }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const base = `/api/sites/${site.id}/share`

  const toggle = useMutation({
    mutationFn: (enable: boolean) => (enable ? api.post<Site>(base) : api.del<Site>(base)),
    onSuccess: updated => qc.setQueryData(['site', String(site.id)], updated),
  })

  const link = site.public_slug ? `${window.location.origin}/share/${site.public_slug}` : ''

  async function copy() {
    await navigator.clipboard.writeText(link)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        aria-expanded={open}
        className="rounded-lg border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
      >
        {site.public_slug ? 'Shared' : 'Share'}
      </button>
      {open && (
        <div className="absolute right-0 z-10 mt-2 w-[min(22rem,calc(100vw-2rem))] rounded-xl border border-zinc-200 bg-white p-4 shadow-lg">
          <h2 className="text-sm font-medium text-zinc-900">Public dashboard</h2>
          <p className="mt-1 text-xs text-zinc-500">
            Anyone with the link can view this dashboard, read-only. Turning it off revokes the
            link; turning it on again creates a new one.
          </p>
          {site.public_slug ? (
            <>
              <div className="mt-3 flex gap-2">
                <input
                  readOnly
                  value={link}
                  aria-label="Public link"
                  onFocus={e => e.target.select()}
                  className="min-w-0 flex-1 rounded-md border border-zinc-200 bg-zinc-50 px-2 py-1 text-xs text-zinc-700"
                />
                <button
                  type="button"
                  onClick={copy}
                  className="rounded-md bg-zinc-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-zinc-800"
                >
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
              <button
                type="button"
                disabled={toggle.isPending}
                onClick={() => toggle.mutate(false)}
                className="mt-3 text-xs font-medium text-red-700 hover:underline disabled:opacity-50 dark:text-red-300"
              >
                Stop sharing
              </button>
            </>
          ) : (
            <button
              type="button"
              disabled={toggle.isPending}
              onClick={() => toggle.mutate(true)}
              className="mt-3 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
            >
              Create public link
            </button>
          )}
          {toggle.error && <div className="mt-3"><ErrorNote message={(toggle.error as Error).message} /></div>}
        </div>
      )}
    </div>
  )
}
