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

/**
 * The timezone every timestamp in the UI is shown in. The API buckets in the
 * same zone (TIMEZONE on the server), so a "day" here is a local day, not a
 * UTC one sliced at 05:30.
 */
export const TZ = 'Asia/Kolkata'
export const TZ_LABEL = 'IST'

/** Short axis label. */
export function bucketLabel(iso: string, interval: string): string {
  const d = new Date(iso)
  return d.toLocaleString(undefined, {
    timeZone: TZ,
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
      ? { timeZone: TZ, month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
      : { timeZone: TZ, weekday: 'short', month: 'short', day: 'numeric' }
  return `${d.toLocaleString(undefined, opts)} ${TZ_LABEL}`
}
