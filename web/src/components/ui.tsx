import { useState, type ReactNode } from 'react'

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <div className={`rounded-xl border border-zinc-200 bg-white ${className}`}>{children}</div>
  )
}

export function CardTitle({ children, aside }: { children: ReactNode; aside?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-zinc-100 px-4 py-3">
      <h2 className="text-sm font-medium text-zinc-900">{children}</h2>
      {aside}
    </div>
  )
}

export function Skeleton({ className = '' }: { className?: string }) {
  return <div aria-hidden className={`animate-pulse rounded bg-zinc-100 ${className}`} />
}

export function ErrorNote({ message }: { message: string }) {
  return (
    <div role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {message}
    </div>
  )
}

/** The script tag for a site, with a copy button that confirms in place. */
export function Snippet({ domain }: { domain: string }) {
  const [copied, setCopied] = useState(false)
  const tag = `<script defer src="${window.location.origin}/script.js" data-domain="${domain}"></script>`

  async function copy() {
    await navigator.clipboard.writeText(tag)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200 bg-zinc-50">
      <div className="flex items-center justify-between border-b border-zinc-200 px-3 py-2">
        <span className="text-xs font-medium text-zinc-500">Paste into your site&rsquo;s &lt;head&gt;</span>
        <button
          type="button"
          onClick={copy}
          className="rounded-md px-2 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-200 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900"
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className="overflow-x-auto px-3 py-3 text-xs leading-relaxed text-zinc-700">
        <code>{tag}</code>
      </pre>
    </div>
  )
}

/** Shown on a site that has never received an event. */
export function WaitingForData({ domain }: { domain: string }) {
  return (
    <Card className="p-6">
      <div className="mx-auto max-w-lg text-center">
        <span className="relative mx-auto mb-4 flex h-3 w-3">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-sky-400 opacity-60" />
          <span className="relative inline-flex h-3 w-3 rounded-full bg-sky-500" />
        </span>
        <h2 className="text-base font-semibold text-zinc-900">Waiting for data</h2>
        <p className="mt-1 text-sm text-zinc-600">
          Nothing has arrived from <span className="font-medium text-zinc-900">{domain}</span> in
          this range. If you haven&rsquo;t installed the snippet yet, add it and load a page —
          events show up within a second.
        </p>
        <div className="mt-5 text-left">
          <Snippet domain={domain} />
        </div>
      </div>
    </Card>
  )
}
