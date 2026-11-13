import { useQuery } from '@tanstack/react-query'
import { api, type Realtime } from '../lib/api'

/** "N current visitors": distinct visitors in the last five minutes, polled. */
export function RealtimeBadge({ base }: { base: string }) {
  const q = useQuery({
    queryKey: ['realtime', base],
    queryFn: () => api.get<Realtime>(`${base}/realtime`),
    refetchInterval: 10_000,
    staleTime: 0,
  })
  if (!q.data) return null
  const n = q.data.visitors
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-white px-3 py-1 text-sm text-zinc-700"
      title="Distinct visitors in the last 5 minutes. Updates every 10 seconds."
      aria-live="polite"
    >
      <span className="relative flex h-2 w-2" aria-hidden>
        {n > 0 && (
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-500 opacity-60" />
        )}
        <span className={`relative inline-flex h-2 w-2 rounded-full ${n > 0 ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
      </span>
      <span className="tabular-nums font-medium text-zinc-900">{n.toLocaleString()}</span>
      {n === 1 ? 'current visitor' : 'current visitors'}
    </span>
  )
}
