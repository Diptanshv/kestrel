import { useMemo } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError, type Breakdown, type PublicSite, type Site, type Summary, type Timeseries } from '../lib/api'
import { RANGES, isRangeKey, rangeParams, type RangeKey } from '../lib/range'
import { FILTER_LABELS, filteredParams, isFilterKey, readFilters, type Filter } from '../lib/filters'
import { bucketLabel, duration, percent } from '../lib/format'
import { Shell } from '../components/Shell'
import { StatTile } from '../components/StatTile'
import { TrafficChart, type ChartDatum } from '../components/TrafficChart'
import { BreakdownCard } from '../components/BreakdownCard'
import { TabbedBreakdown } from '../components/TabbedBreakdown'
import { RealtimeBadge } from '../components/RealtimeBadge'
import { ShareControl } from '../components/ShareControl'
import { Card, ErrorNote, Skeleton, WaitingForData } from '../components/ui'

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

function FilterBar({ filters, onRemove, onClear }: {
  filters: Filter[]
  onRemove: (f: Filter) => void
  onClear: () => void
}) {
  return (
    <div className="mb-6 flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2">
      <span className="text-sm text-zinc-500">Filtered to</span>
      {filters.map(f => (
        <span
          key={f.key}
          className="inline-flex max-w-full items-center gap-1 rounded-md bg-zinc-100 py-0.5 pl-2 pr-1 text-sm text-zinc-800"
        >
          <span className="text-zinc-500">{FILTER_LABELS[f.key]}</span>
          <span className="truncate font-medium">{f.value}</span>
          <button
            type="button"
            onClick={() => onRemove(f)}
            aria-label={`Remove ${FILTER_LABELS[f.key]} filter`}
            className="rounded px-1 text-zinc-500 hover:bg-zinc-200 hover:text-zinc-900"
          >
            &times;
          </button>
        </span>
      ))}
      <button type="button" onClick={onClear} className="text-sm text-zinc-600 underline-offset-4 hover:underline">
        Clear
      </button>
      <span className="w-full text-xs text-zinc-500 sm:ml-auto sm:w-auto">
        Filters cover the last 48 hours — older data is stored per dimension and can&rsquo;t be combined.
      </span>
    </div>
  )
}

/**
 * One dashboard for both audiences: owners reach it at /sites/:id behind a
 * session; anyone with a share link reaches /share/:slug, read-only.
 */
export default function Dashboard() {
  const { id, slug } = useParams<{ id?: string; slug?: string }>()
  const isPublic = slug !== undefined
  const base = isPublic ? `/api/public/${slug}` : `/api/sites/${id}`

  // Range and filters live in the URL, so a reload keeps them and a link
  // carries them.
  const [search, setSearch] = useSearchParams()
  const rangeParam = search.get('range')
  const range: RangeKey = isRangeKey(rangeParam) ? rangeParam : '7d'
  const filters = readFilters(search)
  const filterKey = filters.map(f => `${f.key}=${f.value}`).join('&')
  const filtered = filters.length > 0
  const params = useMemo(
    () => (filtered ? filteredParams(readFilters(new URLSearchParams(filterKey))) : rangeParams(range)),
    [filtered, filterKey, range],
  )
  const scope = filtered ? `filter:${filterKey}` : range

  function updateSearch(mut: (q: URLSearchParams) => void) {
    const next = new URLSearchParams(search)
    mut(next)
    setSearch(next, { replace: true })
  }
  const addFilter = (key: string, value: string) =>
    isFilterKey(key) && updateSearch(q => q.set(key, value))

  // Keep the previous slice on screen while the next one loads.
  const keepPrevious = { placeholderData: <T,>(prev: T | undefined) => prev }

  const site = useQuery({
    queryKey: isPublic ? ['public-site', slug] : ['site', id],
    queryFn: () => api.get<Site | PublicSite>(base),
    retry: (n, err) => !(err instanceof ApiError && err.status === 404) && n < 2,
  })
  const summary = useQuery({
    queryKey: ['summary', base, scope],
    queryFn: () => api.get<Summary>(`${base}/summary?${params}`),
    ...keepPrevious,
  })
  const series = useQuery({
    queryKey: ['timeseries', base, scope],
    queryFn: () => api.get<Timeseries>(`${base}/timeseries?${params}`),
    ...keepPrevious,
  })
  const pages = useQuery({
    queryKey: ['breakdown', base, scope, 'page'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=page&${params}`),
    ...keepPrevious,
  })
  const referrers = useQuery({
    queryKey: ['breakdown', base, scope, 'referrer'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=referrer&${params}`),
    ...keepPrevious,
  })
  const events = useQuery({
    queryKey: ['breakdown', base, scope, 'event'],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=event&${params}`),
    ...keepPrevious,
  })

  if (site.error instanceof ApiError && site.error.status === 404) {
    return (
      <Shell>
        <Card className="mx-auto max-w-md px-6 py-12 text-center">
          <h1 className="text-base font-semibold text-zinc-900">Dashboard not available</h1>
          <p className="mt-1 text-sm text-zinc-500">
            {isPublic
              ? 'This link is invalid or its owner has stopped sharing it.'
              : 'This site does not exist or belongs to another account.'}
          </p>
        </Card>
      </Shell>
    )
  }

  const chartData: ChartDatum[] | undefined = series.data?.points.map(p => ({
    iso: p.bucket,
    label: bucketLabel(p.bucket, series.data.interval),
    visitors: p.visitors,
    pageviews: p.pageviews,
  }))

  const error = summary.error ?? series.error ?? pages.error ?? referrers.error ?? events.error
  const firstLoad = summary.isPending || series.isPending
  const stale = summary.isFetching || series.isFetching
  const prev = summary.data?.previous

  // Nothing at all in the selected window. Without an all-time query we cannot
  // tell "never set up" from "quiet week", so the empty state offers the
  // snippet either way - it is the useful thing in both cases. A filtered
  // view with no matches is just an empty result, not a setup problem.
  const noData =
    !firstLoad &&
    !filtered &&
    summary.data?.pageviews === 0 &&
    !pages.data?.rows.length &&
    !events.data?.rows.length

  const ownedSite = !isPublic && site.data && 'id' in site.data ? site.data : undefined

  return (
    <Shell>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          {isPublic ? (
            <span className="text-sm text-zinc-500">Public dashboard</span>
          ) : (
            <Link
              to="/sites"
              className="text-sm text-zinc-500 underline-offset-4 hover:text-zinc-900 hover:underline"
            >
              &larr; All sites
            </Link>
          )}
          {site.data ? (
            <div className="mt-1 flex flex-wrap items-center gap-3">
              <h1 className="truncate text-2xl font-semibold tracking-tight text-zinc-900">
                {site.data.domain}
              </h1>
              <RealtimeBadge base={base} />
            </div>
          ) : (
            <Skeleton className="mt-2 h-8 w-48" />
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {ownedSite && <ShareControl site={ownedSite} />}
          {filtered ? (
            <span className="rounded-lg border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-600">
              Last 48 hours
            </span>
          ) : (
            <RangePicker
              value={range}
              onChange={k => updateSearch(q => (k === '7d' ? q.delete('range') : q.set('range', k)))}
            />
          )}
        </div>
      </div>

      {filtered && (
        <FilterBar
          filters={filters}
          onRemove={f => updateSearch(q => q.delete(f.key))}
          onClear={() => updateSearch(q => filters.forEach(f => q.delete(f.key)))}
        />
      )}

      {error && <div className="mb-6"><ErrorNote message={(error as Error).message} /></div>}

      {noData && site.data ? (
        isPublic ? (
          <Card className="px-6 py-12 text-center">
            <h2 className="text-base font-medium text-zinc-900">No traffic in this range</h2>
            <p className="mt-1 text-sm text-zinc-500">Try a longer date range.</p>
          </Card>
        ) : (
          <WaitingForData domain={site.data.domain} />
        )
      ) : (
        <>
          <div className="mb-4 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
            <StatTile
              label="Unique visitors"
              value={summary.data?.visitors}
              previous={prev?.visitors}
              series="visitors"
              loading={firstLoad}
              hint="Counted per day and summed across days. Older days are HyperLogLog estimates, accurate to about 1–2%."
            />
            <StatTile label="Pageviews" value={summary.data?.pageviews} previous={prev?.pageviews} series="pageviews" loading={firstLoad} />
            <StatTile
              label="Visits"
              value={summary.data?.visits}
              previous={prev?.visits}
              loading={firstLoad}
              hint="A visit ends after 30 minutes without activity, or at midnight UTC."
            />
            <StatTile
              label="Bounce rate"
              value={summary.data?.bounce_rate}
              previous={prev?.bounce_rate}
              lowerIsBetter
              loading={firstLoad}
              format={percent}
              hint="Visits with a single pageview and no custom event."
            />
            <StatTile
              label="Visit duration"
              value={summary.data?.visit_duration}
              previous={prev?.visit_duration}
              loading={firstLoad}
              format={duration}
              hint="Average time from a visit's first event to its last. Bounces count as 0s."
            />
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
              onSelect={v => addFilter('page', v)}
            />
            <BreakdownCard
              title="Top referrers"
              rows={referrers.data?.rows}
              loading={referrers.isPending}
              stale={referrers.isFetching}
              valueLabel="Referrer"
              emptyLabel="All traffic in this range was direct."
              onSelect={v => addFilter('referrer', v)}
            />
          </div>

          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <TabbedBreakdown
              base={base}
              params={params}
              queryKey={['breakdown', base, scope]}
              title="Audience"
              onSelect={addFilter}
              tabs={[
                { dimension: 'country', label: 'Countries', valueLabel: 'Country', emptyLabel: 'No country data in this range.' },
                { dimension: 'device', label: 'Devices', valueLabel: 'Device', emptyLabel: 'No device data in this range.' },
              ]}
            />
            <TabbedBreakdown
              base={base}
              params={params}
              queryKey={['breakdown', base, scope]}
              title="Technology"
              onSelect={addFilter}
              tabs={[
                { dimension: 'browser', label: 'Browsers', valueLabel: 'Browser', emptyLabel: 'No browser data in this range.' },
                { dimension: 'os', label: 'Operating systems', valueLabel: 'OS', emptyLabel: 'No OS data in this range.' },
              ]}
            />
          </div>

          <div className="mt-4">
            <BreakdownCard
              title="Goals and custom events"
              rows={events.data?.rows}
              loading={events.isPending}
              stale={events.isFetching}
              valueLabel="Event"
              countLabel="Events"
              conversionBase={summary.data?.visitors}
              emptyLabel="No custom events yet. Call kestrel.track('signup') on your site to record one."
            />
          </div>

          <p className="mt-6 text-xs leading-relaxed text-zinc-500">
            Click any page, referrer, country, device, browser or OS to filter by it. Visitors and
            pageviews count page loads only; custom events are listed separately. Times are IST
            (UTC+5:30). Unique visitors are counted per day and summed, so someone visiting on two
            days counts twice — that is the cost of not using cookies. Days older than 48 hours come
            from rollups: their visitor counts are estimates, typically within 1–2%, and rarely-seen
            pages or sources are grouped as &ldquo;(other)&rdquo;. A visit ends after 30 minutes of
            inactivity; a bounce is a visit with one pageview and no custom event. Change badges
            compare with the period of the same length just before.
          </p>
        </>
      )}
    </Shell>
  )
}
