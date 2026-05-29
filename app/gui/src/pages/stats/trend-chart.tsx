import React from 'react'
import { useTranslation } from 'react-i18next'
import { LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts'

import { model } from '@model'
import type { LocalizationKey } from '@/main/i18n'
import { formatJSTDateTime } from '@/helpers/date'

type Props = {
  // User-wide snapshot history (per Capcom /play `battle_stats`, which is
  // a user-level aggregate, not per-character). The chart renders the raw
  // accumulated trend; per-character attribution lives in the detail table.
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

const chartTick = { fill: 'rgba(255,255,255,.78)', fontSize: 11 }
const chartLegend = { color: 'rgba(255,255,255,.86)', fontSize: 12 }
const chartTooltip = {
  background: '#1f1f23',
  border: '1px solid rgba(255,255,255,.18)',
  color: '#f4f4f5'
}

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
    snapshotAt: formatJSTDateTime(s.snapshotAt),
    ...Object.fromEntries(series.map(sr => [sr.key, s[sr.key] as number]))
  }))

  return (
    <div className='mb-6 h-72 w-full'>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 14, right: 20, left: 0, bottom: 24 }}>
          <XAxis
            dataKey='snapshotAt'
            tick={chartTick}
            tickMargin={8}
            stroke='rgba(255,255,255,.3)'
          />
          <YAxis tick={chartTick} stroke='rgba(255,255,255,.3)' />
          <Tooltip contentStyle={chartTooltip} labelStyle={{ color: '#f4f4f5' }} />
          <Legend wrapperStyle={chartLegend} />
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
