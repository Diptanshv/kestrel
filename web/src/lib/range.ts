export type RangeKey = 'today' | '7d' | '30d'

export const RANGES: { key: RangeKey; label: string; description: string }[] = [
  { key: 'today', label: 'Today', description: 'Since 00:00 UTC' },
  { key: '7d', label: '7 days', description: 'Last 7 days' },
  { key: '30d', label: '30 days', description: 'Last 30 days' },
]

export function isRangeKey(v: string | null): v is RangeKey {
  return v === 'today' || v === '7d' || v === '30d'
}

// Computed in UTC because the API buckets in UTC. A "today" that followed the
// browser's zone would not line up with the server's day boundary.
export function rangeParams(key: RangeKey): string {
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
