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
}: {
  label: string
  value: number | undefined
  /** Ties the tile to a chart line. Omit for metrics the chart does not plot. */
  series?: keyof typeof SERIES
  loading: boolean
  format?: (n: number) => string
  hint?: string
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
        <div
          className="mt-1 text-3xl font-semibold text-zinc-900"
          title={full(value)}
        >
          {format(value)}
        </div>
      )}
    </div>
  )
}
