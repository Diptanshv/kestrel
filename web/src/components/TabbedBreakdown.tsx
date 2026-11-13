import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Breakdown } from '../lib/api'
import { BreakdownCard } from './BreakdownCard'

type Tab = { dimension: string; label: string; valueLabel: string; emptyLabel: string }

export function TabbedBreakdown({
  base,
  params,
  queryKey,
  title,
  tabs,
}: {
  base: string
  params: string
  queryKey: unknown[]
  title: string
  tabs: Tab[]
}) {
  const [active, setActive] = useState(tabs[0])

  const q = useQuery({
    queryKey: [...queryKey, active.dimension],
    queryFn: () => api.get<Breakdown>(`${base}/breakdown?dimension=${active.dimension}&${params}`),
    placeholderData: prev => prev,
  })

  return (
    <div>
      <div role="tablist" aria-label={title} className="mb-2 flex gap-1">
        {tabs.map(t => (
          <button
            key={t.dimension}
            role="tab"
            aria-selected={t.dimension === active.dimension}
            onClick={() => setActive(t)}
            className={`rounded-md px-2.5 py-1 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900 ${
              t.dimension === active.dimension
                ? 'bg-zinc-900 text-white'
                : 'text-zinc-600 hover:bg-zinc-100'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      <BreakdownCard
        title={active.label}
        rows={q.data?.rows}
        loading={q.isPending}
        stale={q.isFetching}
        valueLabel={active.valueLabel}
        emptyLabel={active.emptyLabel}
      />
    </div>
  )
}
