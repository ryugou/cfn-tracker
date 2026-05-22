import React from 'react'

import { cn } from '@/helpers/cn'

type Props = {
  label: string
  value: string
  delta: { text: string; direction: 'up' | 'down' | 'flat' }
  tooltip?: string
}

export function KpiCard({ label, value, delta, tooltip }: Props) {
  const arrow = delta.direction === 'up' ? '↑' : delta.direction === 'down' ? '↓' : ''
  return (
    <div className='rounded-lg bg-zinc-800/80 p-3 min-w-[120px]' title={tooltip}>
      <div className='text-xs text-white/60 mb-1'>{label}</div>
      <div className='text-xl text-white tabular-nums'>{value}</div>
      <div
        className={cn(
          'text-xs mt-1',
          delta.direction === 'up' && 'text-emerald-400',
          delta.direction === 'down' && 'text-rose-400',
          delta.direction === 'flat' && 'text-white/40'
        )}
      >
        {delta.text} {arrow}
      </div>
    </div>
  )
}
