import { SERIES } from '../lib/palette'
import { compact, full } from '../lib/format'
import { Skeleton } from './ui'

/**
 * Stat tile: label, value, and a colour key tying it to its series in the
 * chart. The value uses proportional figures - tabular-nums is for columns of
 * numbers, not standalone display values.
 */
export function StatTile({
  label,
  value,
  series,
  loading,
  format = compact,
  hint,
  previous,
  lowerIsBetter = false,
}: {
  label: string
  value: number | undefined
  /** Ties the tile to a chart line. Omit for metrics the chart does not plot. */
  series?: keyof typeof SERIES
  loading: boolean
  format?: (n: number) => string
  hint?: string
  /** The same metric for the previous period; drives the change badge. */
  previous?: number | null
  lowerIsBetter?: boolean
}) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-4">
      <div className="flex items-center gap-2">
        {series && (
          <span
            aria-hidden
            className="h-2.5 w-2.5 shrink-0 rounded-full"
            style={{ backgroundColor: SERIES[series] }}
          />
        )}
        <span className="text-sm text-zinc-500" title={hint}>{label}</span>
      </div>
      {loading || value === undefined ? (
        <Skeleton className="mt-2 h-8 w-20" />
      ) : (
        <div className="mt-1 flex flex-wrap items-baseline gap-x-2">
          <div className="text-3xl font-semibold text-zinc-900" title={full(value)}>
            {format(value)}
          </div>
          <Delta value={value} previous={previous} lowerIsBetter={lowerIsBetter} />
        </div>
      )}
    </div>
  )
}

/**
 * Percent change versus the previous period. Colour encodes whether the
 * change is good (direction x lowerIsBetter), and the arrow carries the
 * direction itself, so colour is never the only signal.
 */
function Delta({
  value,
  previous,
  lowerIsBetter,
}: {
  value: number
  previous: number | null | undefined
  lowerIsBetter: boolean
}) {
  if (previous === undefined || previous === null) return null
  if (previous === 0) {
    return value > 0 ? <span className="text-xs text-zinc-500">new</span> : null
  }
  const pct = ((value - previous) / previous) * 100
  if (Math.abs(pct) < 0.5) {
    return <span className="text-xs text-zinc-500" title="vs previous period">no change</span>
  }
  const up = pct > 0
  const good = up !== lowerIsBetter
  return (
    <span
      className="text-xs font-medium tabular-nums"
      style={{ color: good ? 'var(--delta-good)' : 'var(--delta-bad)' }}
      title={`vs previous period (${full(previous)})`}
    >
      <span aria-hidden>{up ? '↑' : '↓'}</span>
      <span className="sr-only">{up ? 'up' : 'down'}</span> {Math.abs(pct).toFixed(Math.abs(pct) < 10 ? 1 : 0)}%
    </span>
  )
}
