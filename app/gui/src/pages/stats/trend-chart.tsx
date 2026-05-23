import React from 'react'
import { useTranslation } from 'react-i18next'
import { LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts'

import { model } from '@model'
import type { LocalizationKey } from '@/main/i18n'

type Props = {
  history: model.PlayStatsSnapshot[]
}

const series: Array<{
  key: keyof model.PlayStatsSnapshot
  labelKey: LocalizationKey
  color: string
}> = [
  { key: 'driveImpact', labelKey: 'kpiDriveImpact', color: '#34d399' },
  { key: 'receivedDriveImpact', labelKey: 'kpiReceivedDi', color: '#f87171' },
  { key: 'justParry', labelKey: 'kpiJustParry', color: '#a78bfa' },
  { key: 'throwTech', labelKey: 'kpiThrowTech', color: '#fbbf24' },
  { key: 'cornerTime', labelKey: 'kpiCornerTime', color: '#60a5fa' },
  { key: 'gaugeRateSALv3', labelKey: 'kpiSaLv3', color: '#fb923c' }
]

export function TrendChart({ history }: Props) {
  const { t } = useTranslation()

  if (history.length < 2) {
    return (
      <div className='flex h-64 items-center justify-center text-white/40'>
        — {t('statsNeedMorePoints')} —
      </div>
    )
  }

  const data = history.map(s => ({
    snapshotAt: s.snapshotAt,
    ...Object.fromEntries(series.map(sr => [sr.key, s[sr.key] as number]))
  }))

  return (
    <div className='mb-6 h-64 w-full'>
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
            <Line
              key={s.key}
              type='monotone'
              dataKey={s.key}
              name={t(s.labelKey)}
              stroke={s.color}
              dot={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
