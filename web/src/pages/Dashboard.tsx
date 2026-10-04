import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from 'recharts'
import { api, type Breakdown, type Site, type Summary, type Timeseries } from '../lib/api'

type RangeKey = 'today' | '7d' | '30d'

const RANGES: { key: RangeKey; label: string }[] = [
  { key: 'today', label: 'Today' },
  { key: '7d', label: '7 days' },
  { key: '30d', label: '30 days' },
]

// Ranges are computed in UTC because the API buckets in UTC. A "today" that
// follows the browser's zone would not line up with the server's day boundary.
function rangeParams(key: RangeKey): string {
  const now = new Date()
  const from = new Date(now)
  if (key === 'today') {
    from.setUTCHours(0, 0, 0, 0)
  } else {
    from.setUTCDate(from.getUTCDate() - (key === '7d' ? 7 : 30))
  }
  const interval = key === 'today' ? 'hour' : 'day'
  return `from=${from.toISOString()}&to=${now.toISOString()}&interval=${interval}`
}

function StatTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-4">
      <div className="text-sm text-zinc-500">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-zinc-900">
        {value.toLocaleString()}
      </div>
    </div>
  )
}

function BreakdownTable({ title, rows, emptyLabel, countLabel = 'Pageviews' }: {
  title: string
  rows: Breakdown['rows'] | undefined
  emptyLabel: string
  countLabel?: string
}) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white">
      <h2 className="border-b border-zinc-200 px-4 py-3 text-sm font-medium text-zinc-900">{title}</h2>
      {!rows?.length ? (
        <p className="px-4 py-6 text-sm text-zinc-500">{emptyLabel}</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs uppercase tracking-wide text-zinc-500">
              <th className="px-4 py-2 font-medium">Value</th>
              <th className="px-4 py-2 text-right font-medium">Visitors</th>
              <th className="px-4 py-2 text-right font-medium">{countLabel}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(row => (
              <tr key={row.value} className="border-t border-zinc-100">
                <td className="max-w-[20rem] truncate px-4 py-2 text-zinc-800">{row.value}</td>
                <td className="px-4 py-2 text-right tabular-nums text-zinc-600">{row.visitors.toLocaleString()}</td>
                <td className="px-4 py-2 text-right tabular-nums text-zinc-600">{row.pageviews.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

export default function Dashboard() {
  const { id } = useParams<{ id: string }>()
  const [range, setRange] = useState<RangeKey>('7d')
  const params = useMemo(() => rangeParams(range), [range])
  const base = `/api/sites/${id}`

  const site = useQuery({ queryKey: ['site', id], queryFn: () => api.get<Site>(base) })
  const summary = useQuery({
    queryKey: ['summary', id, range],
    queryFn: () => api.get<Summary>(`${base}/summary?${params}`),
  })
  const series = useQuery({
    queryKey: ['timeseries', id, range],
    queryFn: () => api.get<Timeseries>(`${base}/timeseries?${params}`),
  })
  const pages = useQuery({
    queryKey: ['breakdown', id, range, 'page'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=page&${params}`),
  })
  const referrers = useQuery({
    queryKey: ['breakdown', id, range, 'referrer'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=referrer&${params}`),
  })
  const events = useQuery({
    queryKey: ['breakdown', id, range, 'event'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=event&${params}`),
  })

  const chartData = series.data?.points.map(p => ({
    label: new Date(p.bucket).toLocaleString(undefined, {
      timeZone: 'UTC',
      ...(series.data?.interval === 'hour'
        ? { hour: '2-digit', minute: '2-digit' }
        : { month: 'short', day: 'numeric' }),
    }),
    visitors: p.visitors,
    pageviews: p.pageviews,
  })) ?? []

  const error = summary.error ?? series.error ?? pages.error ?? referrers.error ?? events.error

  return (
    <div className="mx-auto max-w-5xl px-4 py-10">
      <header className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div>
          <Link to="/sites" className="text-sm text-zinc-600 underline">All sites</Link>
          <h1 className="mt-1 text-2xl font-semibold text-zinc-900">{site.data?.domain ?? '...'}</h1>
        </div>
        <div className="flex gap-1 rounded-lg border border-zinc-200 bg-white p-1">
          {RANGES.map(r => (
            <button
              key={r.key} onClick={() => setRange(r.key)}
              className={`rounded-md px-3 py-1.5 text-sm ${
                range === r.key ? 'bg-zinc-900 text-white' : 'text-zinc-600 hover:bg-zinc-50'
              }`}
            >
              {r.label}
            </button>
          ))}
        </div>
      </header>

      {error && <p className="mb-6 text-sm text-red-600">{(error as Error).message}</p>}

      <div className="mb-6 grid grid-cols-2 gap-4">
        <StatTile label="Unique visitors" value={summary.data?.visitors ?? 0} />
        <StatTile label="Pageviews" value={summary.data?.pageviews ?? 0} />
      </div>

      <div className="mb-6 rounded-xl border border-zinc-200 bg-white p-4">
        <h2 className="mb-4 text-sm font-medium text-zinc-900">
          Visitors and pageviews{series.data ? ` (per ${series.data.interval}, UTC)` : ''}
        </h2>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 4, right: 8, bottom: 0, left: -16 }}>
              <CartesianGrid stroke="#f1f1f4" vertical={false} />
              <XAxis dataKey="label" tick={{ fontSize: 11, fill: '#71717b' }} tickLine={false} axisLine={false} minTickGap={24} />
              <YAxis tick={{ fontSize: 11, fill: '#71717b' }} tickLine={false} axisLine={false} allowDecimals={false} />
              <Tooltip />
              <Line type="monotone" dataKey="visitors" stroke="#18181b" strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="pageviews" stroke="#a1a1aa" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <BreakdownTable title="Top pages" rows={pages.data?.rows} emptyLabel="No pageviews in this range yet." />
        <BreakdownTable title="Top referrers" rows={referrers.data?.rows} emptyLabel="All traffic in this range was direct." />
      </div>

      <div className="mt-4">
        <BreakdownTable
          title="Custom events"
          rows={events.data?.rows}
          countLabel="Events"
          emptyLabel="No custom events yet. Call kestrel.track('signup') from your site."
        />
      </div>

      <p className="mt-6 text-xs text-zinc-500">
        Visitors and pageviews count page loads only; custom events are listed
        separately. Bounce rate and visit duration need session tracking
        (Phase 7). Countries, browsers and devices need enrichment (Phase 5).
      </p>
    </div>
  )
}