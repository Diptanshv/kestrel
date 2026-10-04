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
}: {
  label: string
  value: number | undefined
  series: keyof typeof SERIES
  loading: boolean
}) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-4">
      <div className="flex items-center gap-2">
        <span
          aria-hidden
          className="h-2.5 w-2.5 shrink-0 rounded-full"
          style={{ backgroundColor: SERIES[series] }}
        />
        <span className="text-sm text-zinc-500">{label}</span>
      </div>
      {loading || value === undefined ? (
        <Skeleton className="mt-2 h-8 w-20" />
      ) : (
        <div
          className="mt-1 text-3xl font-semibold text-zinc-900"
          title={full(value)}
        >
          {compact(value)}
        </div>
      )}
    </div>
  )
}
