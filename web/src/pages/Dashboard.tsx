import { useMemo } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, type Breakdown, type Site, type Summary, type Timeseries } from '../lib/api'
import { RANGES, isRangeKey, rangeParams, type RangeKey } from '../lib/range'
import { bucketLabel } from '../lib/format'
import { Shell } from '../components/Shell'
import { StatTile } from '../components/StatTile'
import { TrafficChart, type ChartDatum } from '../components/TrafficChart'
import { BreakdownCard } from '../components/BreakdownCard'
import { ErrorNote, Skeleton, WaitingForData } from '../components/ui'

function RangePicker({ value, onChange }: { value: RangeKey; onChange: (k: RangeKey) => void }) {
  return (
    // One row of filters above everything they scope, date range first.
    <div role="group" aria-label="Date range" className="flex gap-1 rounded-lg border border-zinc-200 bg-white p-1">
      {RANGES.map(r => {
        const selected = value === r.key
        return (
          <button
            key={r.key}
            type="button"
            onClick={() => onChange(r.key)}
            aria-pressed={selected}
            title={r.description}
            className={`rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900 ${
              selected ? 'bg-zinc-900 text-white' : 'text-zinc-600 hover:bg-zinc-100'
            }`}
          >
            {r.label}
          </button>
        )
      })}
    </div>
  )
}

export default function Dashboard() {
  const { id } = useParams<{ id: string }>()
  // The range lives in the URL, so a reload keeps it and a link can carry it.
  const [search, setSearch] = useSearchParams()
  const rangeParam = search.get('range')
  const range: RangeKey = isRangeKey(rangeParam) ? rangeParam : '7d'
  const params = useMemo(() => rangeParams(range), [range])
  const base = `/api/sites/${id}`

  // Keep the previous slice on screen while the next one loads.
  const keepPrevious = { placeholderData: <T,>(prev: T | undefined) => prev }

  const site = useQuery({ queryKey: ['site', id], queryFn: () => api.get<Site>(base) })
  const summary = useQuery({
    queryKey: ['summary', id, range],
    queryFn: () => api.get<Summary>(`${base}/summary?${params}`),
    ...keepPrevious,
  })
  const series = useQuery({
    queryKey: ['timeseries', id, range],
    queryFn: () => api.get<Timeseries>(`${base}/timeseries?${params}`),
    ...keepPrevious,
  })
  const pages = useQuery({
    queryKey: ['breakdown', id, range, 'page'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=page&${params}`),
    ...keepPrevious,
  })
  const referrers = useQuery({
    queryKey: ['breakdown', id, range, 'referrer'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=referrer&${params}`),
    ...keepPrevious,
  })
  const events = useQuery({
    queryKey: ['breakdown', id, range, 'event'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=event&${params}`),
    ...keepPrevious,
  })

  const chartData: ChartDatum[] | undefined = series.data?.points.map(p => ({
    iso: p.bucket,
    label: bucketLabel(p.bucket, series.data.interval),
    visitors: p.visitors,
    pageviews: p.pageviews,
  }))

  const error = summary.error ?? series.error ?? pages.error ?? referrers.error ?? events.error
  const firstLoad = summary.isPending || series.isPending
  const stale = summary.isFetching || series.isFetching

  // Nothing at all in the selected window. Without an all-time query we cannot
  // tell "never set up" from "quiet week", so the empty state offers the
  // snippet either way - it is the useful thing in both cases.
  const noData =
    !firstLoad &&
    summary.data?.pageviews === 0 &&
    !pages.data?.rows.length &&
    !events.data?.rows.length

  return (
    <Shell>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <Link
            to="/sites"
            className="text-sm text-zinc-500 underline-offset-4 hover:text-zinc-900 hover:underline"
          >
            &larr; All sites
          </Link>
          {site.data ? (
            <h1 className="mt-1 truncate text-2xl font-semibold tracking-tight text-zinc-900">
              {site.data.domain}
            </h1>
          ) : (
            <Skeleton className="mt-2 h-8 w-48" />
          )}
        </div>
        <RangePicker
          value={range}
          onChange={k => setSearch(k === '7d' ? {} : { range: k }, { replace: true })}
        />
      </div>

      {error && <div className="mb-6"><ErrorNote message={(error as Error).message} /></div>}

      {noData && site.data ? (
        <WaitingForData domain={site.data.domain} />
      ) : (
        <>
          <div className="mb-4 grid grid-cols-2 gap-4">
            <StatTile label="Unique visitors" value={summary.data?.visitors} series="visitors" loading={firstLoad} />
            <StatTile label="Pageviews" value={summary.data?.pageviews} series="pageviews" loading={firstLoad} />
          </div>

          <div className="mb-4">
            <TrafficChart
              points={chartData}
              interval={series.data?.interval ?? 'day'}
              loading={firstLoad}
              stale={stale}
            />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <BreakdownCard
              title="Top pages"
              rows={pages.data?.rows}
              loading={pages.isPending}
              stale={pages.isFetching}
              emptyLabel="No pageviews in this range."
            />
            <BreakdownCard
              title="Top referrers"
              rows={referrers.data?.rows}
              loading={referrers.isPending}
              stale={referrers.isFetching}
              valueLabel="Referrer"
              emptyLabel="All traffic in this range was direct."
            />
          </div>

          <div className="mt-4">
            <BreakdownCard
              title="Custom events"
              rows={events.data?.rows}
              loading={events.isPending}
              stale={events.isFetching}
              valueLabel="Event"
              countLabel="Events"
              emptyLabel="No custom events. Call kestrel.track('signup') from your site to record one."
            />
          </div>

          <p className="mt-6 text-xs leading-relaxed text-zinc-500">
            Visitors and pageviews count page loads only; custom events are listed separately.
            Times are UTC. Bounce rate and visit duration need session tracking; countries,
            browsers and devices need enrichment.
          </p>
        </>
      )}
    </Shell>
  )
}
