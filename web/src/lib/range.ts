import { TZ } from './format'

export type RangeKey = 'today' | '7d' | '30d'

export const RANGES: { key: RangeKey; label: string; description: string }[] = [
  { key: 'today', label: 'Today', description: 'Since midnight IST' },
  { key: '7d', label: '7 days', description: 'Last 7 days' },
  { key: '30d', label: '30 days', description: 'Last 30 days' },
]

export function isRangeKey(v: string | null): v is RangeKey {
  return v === 'today' || v === '7d' || v === '30d'
}

/**
 * Midnight in TZ, as an absolute instant. Built from the zone's own calendar
 * fields rather than the browser's, so the window matches the server's buckets
 * whatever zone the reader happens to be in.
 */
function startOfDayInTZ(d: Date): Date {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: TZ,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).formatToParts(d)
  const get = (t: string) => Number(parts.find(p => p.type === t)?.value ?? 0)
  // How far into its local day the instant is, subtracted off to reach midnight.
  const intoDay = (get('hour') % 24) * 3600_000 + get('minute') * 60_000 + get('second') * 1000
  return new Date(d.getTime() - intoDay - (d.getTime() % 1000))
}

export function rangeParams(key: RangeKey): string {
  const now = new Date()
  let from: Date
  if (key === 'today') {
    from = startOfDayInTZ(now)
  } else {
    const days = key === '7d' ? 7 : 30
    from = new Date(startOfDayInTZ(now).getTime() - days * 86400_000)
  }
  const interval = key === 'today' ? 'hour' : 'day'
  return `from=${from.toISOString()}&to=${now.toISOString()}&interval=${interval}`
}
