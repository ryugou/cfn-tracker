import React from 'react'
import { LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts'

import { model } from '@model'

type Props = {
  history: model.PlayStatsSnapshot[]
}

const series: Array<{
  key: keyof model.PlayStatsSnapshot
  label: string
  color: string
}> = [
  { key: 'driveImpact',         label: 'DI 命中',   color: '#34d399' },
  { key: 'receivedDriveImpact', label: 'DI 被弾',   color: '#f87171' },
  { key: 'justParry',           label: 'ジャパリ',  color: '#a78bfa' },
  { key: 'throwTech',           label: '投げ抜け',  color: '#fbbf24' },
  { key: 'cornerTime',          label: '壁際秒',    color: '#60a5fa' },
  { key: 'gaugeRateSALv3',      label: 'SA Lv3 %',  color: '#fb923c' },
]

export function TrendChart({ history }: Props) {
  const data = history.map(s => ({
    snapshotAt: s.snapshotAt,
    ...Object.fromEntries(series.map(sr => [sr.key, s[sr.key] as number])),
  }))

  if (history.length < 2) {
    return (
      <div className='h-64 flex items-center justify-center text-white/40'>
        — 2 points required for a trend —
      </div>
    )
  }

  return (
    <div className='h-64 w-full mb-6'>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
          <XAxis dataKey='snapshotAt' tick={{ fontSize: 10 }} stroke='#888' />
          <YAxis tick={{ fontSize: 10 }} stroke='#888' />
          <Tooltip
            contentStyle={{ background: '#1f1f23', border: '1px solid #333' }}
            labelStyle={{ color: '#aaa' }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {series.map(s => (
            <Line key={s.key} type='monotone' dataKey={s.key} name={s.label} stroke={s.color} dot={false} />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
