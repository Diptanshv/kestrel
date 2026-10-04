import type { Breakdown } from '../lib/api'
import { SERIES } from '../lib/palette'
import { full } from '../lib/format'
import { Card, CardTitle, Skeleton } from './ui'

/**
 * Top-N table with a share bar behind each row. The bar is a magnitude
 * encoding for one series, so every row is the same hue - darkening by value
 * would double-encode the length and fail the categorical checks.
 */
export function BreakdownCard({
  title,
  rows,
  loading,
  stale,
  emptyLabel,
  countLabel = 'Pageviews',
  valueLabel = 'Page',
}: {
  title: string
  rows: Breakdown['rows'] | undefined
  loading: boolean
  stale: boolean
  emptyLabel: string
  countLabel?: string
  valueLabel?: string
}) {
  const max = rows?.reduce((m, r) => Math.max(m, r.pageviews), 0) ?? 0

  return (
    <Card>
      <CardTitle>{title}</CardTitle>

      {loading ? (
        <div className="space-y-2 p-4">
          {[0, 1, 2, 3].map(i => (
            <Skeleton key={i} className="h-7 w-full" />
          ))}
        </div>
      ) : !rows?.length ? (
        <p className="px-4 py-8 text-center text-sm text-zinc-500">{emptyLabel}</p>
      ) : (
        <div className={`overflow-x-auto transition-opacity ${stale ? 'opacity-50' : ''}`}>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-xs font-normal text-zinc-500">
                <th scope="col" className="px-4 py-2 text-left font-medium">
                  {valueLabel}
                </th>
                <th scope="col" className="w-20 py-2 pr-2 text-right font-medium">
                  Visitors
                </th>
                <th scope="col" className="w-20 py-2 pr-4 text-right font-medium">
                  {countLabel}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map(row => (
                <tr key={row.value} className="group">
                  <td className="relative py-0 pl-4 pr-2">
                    {/* The share bar sits behind the label rather than in its own
                        column, so the row reads as one object. */}
                    <span
                      aria-hidden
                      className="absolute inset-y-1 left-2 rounded-r-sm rounded-l-none"
                      style={{
                        width: `${max ? (row.pageviews / max) * 100 : 0}%`,
                        backgroundColor: SERIES.visitors,
                        opacity: 0.1,
                      }}
                    />
                    <span
                      className="relative block max-w-[18rem] truncate py-2 text-zinc-800"
                      title={row.value}
                    >
                      {row.value}
                    </span>
                  </td>
                  <td className="w-20 py-2 pr-2 text-right tabular-nums text-zinc-500">
                    {full(row.visitors)}
                  </td>
                  <td className="w-20 py-2 pr-4 text-right font-medium tabular-nums text-zinc-900">
                    {full(row.pageviews)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  )
}
