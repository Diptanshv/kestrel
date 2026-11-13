/** Breakdown dimensions the API can filter by, in display order. */
export const FILTER_KEYS = ['page', 'referrer', 'country', 'browser', 'os', 'device'] as const
export type FilterKey = (typeof FILTER_KEYS)[number]
export type Filter = { key: FilterKey; value: string }

export const FILTER_LABELS: Record<FilterKey, string> = {
  page: 'Page',
  referrer: 'Referrer',
  country: 'Country',
  browser: 'Browser',
  os: 'OS',
  device: 'Device',
}

export function isFilterKey(k: string): k is FilterKey {
  return (FILTER_KEYS as readonly string[]).includes(k)
}

export function readFilters(search: URLSearchParams): Filter[] {
  return FILTER_KEYS.flatMap(key => {
    const value = search.get(key)
    return value ? [{ key, value }] : []
  })
}

/**
 * Filters only work on raw events, which are kept for 48 hours, so a filtered
 * view always covers exactly that window in hourly buckets.
 */
export function filteredParams(filters: Filter[]): string {
  const now = new Date()
  const from = new Date(now.getTime() - 48 * 3600_000)
  const q = new URLSearchParams({ from: from.toISOString(), to: now.toISOString(), interval: 'hour' })
  for (const f of filters) q.set(f.key, f.value)
  return q.toString()
}
