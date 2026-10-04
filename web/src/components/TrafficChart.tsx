import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { AXIS_TEXT, GRID, SERIES, SURFACE } from '../lib/palette'
import { bucketTitle, compact, full } from '../lib/format'
import { Skeleton } from './ui'

export type ChartDatum = {
  iso: string
  label: string
  visitors: number
  pageviews: number
}

// Typed against only what this tooltip reads, rather than threading Recharts'
// generics through the Tooltip element.
type TooltipProps = {
  active?: boolean
  payload?: readonly { payload: ChartDatum }[]
  interval: string
}

function ChartTooltip({ active, payload, interval }: TooltipProps) {
  if (!active || !payload?.length) return null
  const datum = payload[0].payload

  // Values lead, labels follow: the reader already knows the series and wants
  // the number. Line keys rather than filled boxes at this density.
  const rows = [
    { name: 'Visitors', value: datum.visitors, color: SERIES.visitors },
    { name: 'Pageviews', value: datum.pageviews, color: SERIES.pageviews },
  ]

  return (
    <div className="rounded-lg border border-zinc-200 bg-white px-3 py-2 shadow-lg">
      <div className="mb-1.5 text-xs text-zinc-500">{bucketTitle(datum.iso, interval)}</div>
      <table className="w-full">
        <tbody>
          {rows.map(row => (
            <tr key={row.name}>
              <td className="pr-3">
                <span className="flex items-center gap-2">
                  <span
                    aria-hidden
                    className="h-0.5 w-3 rounded-full"
                    style={{ backgroundColor: row.color }}
                  />
                  <span className="text-xs text-zinc-500">{row.name}</span>
                </span>
              </td>
              <td className="text-right text-sm font-semibold tabular-nums text-zinc-900">
                {full(row.value)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function LegendKey({ name, color, mark }: { name: string; color: string; mark: 'area' | 'line' }) {
  return (
    <span className="flex items-center gap-2">
      <span
        aria-hidden
        className={mark === 'area' ? 'h-2.5 w-3 rounded-sm' : 'h-0.5 w-3 rounded-full'}
        style={{ backgroundColor: color, opacity: mark === 'area' ? 0.9 : 1 }}
      />
      <span className="text-xs text-zinc-600">{name}</span>
    </span>
  )
}

export function TrafficChart({
  points,
  interval,
  loading,
  stale,
}: {
  points: ChartDatum[] | undefined
  interval: string
  loading: boolean
  stale: boolean
}) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-4">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-medium text-zinc-900">Traffic over time</h2>
        {/* A legend is always present for two or more series, so identity is
            never carried by colour alone. */}
        <div className="flex items-center gap-4">
          <LegendKey name="Visitors" color={SERIES.visitors} mark="area" />
          <LegendKey name="Pageviews" color={SERIES.pageviews} mark="line" />
        </div>
      </div>

      {loading || !points ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        // Refetch keeps the frame: the previous render stays, dimmed, instead of
        // collapsing to a skeleton and jumping the layout.
        <div className={`h-64 transition-opacity ${stale ? 'opacity-50' : 'opacity-100'}`}>
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: -18 }}>
              <CartesianGrid stroke={GRID} strokeWidth={1} vertical={false} />
              <XAxis
                dataKey="label"
                tick={{ fontSize: 11, fill: AXIS_TEXT }}
                tickLine={false}
                axisLine={{ stroke: GRID }}
                minTickGap={28}
              />
              <YAxis
                tick={{ fontSize: 11, fill: AXIS_TEXT }}
                tickLine={false}
                axisLine={false}
                allowDecimals={false}
                width={48}
                tickFormatter={(v: number | string) => compact(Number(v))}
              />
              <Tooltip
                cursor={{ stroke: AXIS_TEXT, strokeWidth: 1 }}
                content={props => (
                  <ChartTooltip
                    active={props.active}
                    payload={props.payload as unknown as TooltipProps['payload']}
                    interval={interval}
                  />
                )}
              />
              {/* Linear, not monotone: a smooth curve between two sparse
                  buckets draws growth that never happened. */}
              <Area
                type="linear"
                dataKey="visitors"
                stroke={SERIES.visitors}
                strokeWidth={2}
                fill={SERIES.visitors}
                fillOpacity={0.1}
                activeDot={{ r: 4, strokeWidth: 2, stroke: SURFACE }}
              />
              <Line
                type="linear"
                dataKey="pageviews"
                stroke={SERIES.pageviews}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4, strokeWidth: 2, stroke: SURFACE }}
              />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
