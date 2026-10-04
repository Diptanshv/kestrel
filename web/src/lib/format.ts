// Proportional figures for standalone values; the caller adds tabular-nums only
// where numbers stack in a column.
export function compact(n: number): string {
  if (n < 1000) return n.toLocaleString()
  return new Intl.NumberFormat(undefined, {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(n)
}

export function full(n: number): string {
  return n.toLocaleString()
}

/** Short axis label. UTC, because the API buckets in UTC. */
export function bucketLabel(iso: string, interval: string): string {
  const d = new Date(iso)
  return d.toLocaleString(undefined, {
    timeZone: 'UTC',
    ...(interval === 'hour'
      ? { hour: '2-digit', minute: '2-digit' }
      : { month: 'short', day: 'numeric' }),
  })
}

/** Full label for the tooltip header, where there is room to be unambiguous. */
export function bucketTitle(iso: string, interval: string): string {
  const d = new Date(iso)
  const opts: Intl.DateTimeFormatOptions =
    interval === 'hour'
      ? { timeZone: 'UTC', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
      : { timeZone: 'UTC', weekday: 'short', month: 'short', day: 'numeric' }
  return `${d.toLocaleString(undefined, opts)} UTC`
}
