import React from 'react'

import { cn } from '@/helpers/cn'

type Props = {
  label: string
  value: string
  delta: { text: string; direction: 'up' | 'down' | 'flat' }
}

export function KpiCard({ label, value, delta }: Props) {
  const arrow = delta.direction === 'up' ? '↑' : delta.direction === 'down' ? '↓' : ''
  return (
    <div className='min-w-[120px] rounded-lg bg-zinc-800/80 p-3'>
      <div className='mb-1 text-xs text-white/60'>{label}</div>
      <div className='text-xl text-white tabular-nums'>{value}</div>
      <div
        className={cn(
          'mt-1 text-xs',
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
