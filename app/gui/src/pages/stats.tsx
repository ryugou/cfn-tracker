import React from 'react'
import { useTranslation } from 'react-i18next'

import * as Page from '@/ui/page'
import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import { GetUsers, GetPlayStatsCharacters, GetPlayStatsHistory } from '@cmd/CommandHandler'
import { model } from '@model'
import { KpiCard } from './stats/kpi-card'
import { formatRate, formatPerMatchCount, formatSeconds, formatDelta } from './stats/formatters'
import { TrendChart } from './stats/trend-chart'
import { DetailTable } from './stats/detail-table'

type Period = '7' | '30' | 'all'

type Snapshot = model.PlayStatsSnapshot

const kpiSpecs: Array<{
  key: string
  value: (s: Snapshot) => number
  format: (n: number) => string
}> = [
  { key: 'kpiDriveImpact', value: s => s.driveImpact, format: formatPerMatchCount },
  { key: 'kpiReceivedDi', value: s => s.receivedDriveImpact, format: formatPerMatchCount },
  { key: 'kpiJustParry', value: s => s.justParry, format: formatPerMatchCount },
  { key: 'kpiThrowTech', value: s => s.throwTech, format: formatPerMatchCount },
  { key: 'kpiCornerTime', value: s => s.cornerTime, format: formatSeconds },
  { key: 'kpiSaLv3', value: s => s.gaugeRateSALv3, format: formatRate }
]

export function StatsPage() {
  const { t } = useTranslation()
  const trackingUser = TrackingMachineContext.useSelector(s => s.context.user)
  const game = AuthMachineContext.useSelector(s => s.context.game)

  const [users, setUsers] = React.useState<model.User[]>([])
  const [characters, setCharacters] = React.useState<string[]>([])
  const [selectedUser, setSelectedUser] = React.useState<string>('')
  const [selectedChar, setSelectedChar] = React.useState<string>('')
  const [period, setPeriod] = React.useState<Period>('30')
  const [history, setHistory] = React.useState<model.PlayStatsSnapshot[]>([])
  const [loading, setLoading] = React.useState(false)

  React.useEffect(() => {
    GetUsers().then(us => {
      setUsers(us ?? [])
      if (!selectedUser && trackingUser) {
        setSelectedUser(trackingUser.code)
      } else if (!selectedUser && us?.length) {
        setSelectedUser(us[0].code)
      }
    })
  }, [trackingUser])

  React.useEffect(() => {
    if (!selectedUser) return
    GetPlayStatsCharacters(selectedUser).then(cs => {
      setCharacters(cs ?? [])
      if (cs?.length && !selectedChar) setSelectedChar(cs[0])
    })
  }, [selectedUser])

  React.useEffect(() => {
    if (!selectedUser || !selectedChar) return
    setLoading(true)
    const today = new Date()
    let from = ''
    if (period === '7' || period === '30') {
      const days = period === '7' ? 7 : 30
      const d = new Date(today.getTime() - days * 86400000)
      from = d.toISOString().slice(0, 10)
    }
    GetPlayStatsHistory(selectedUser, selectedChar, from, '', 0)
      .then(rows => setHistory(rows ?? []))
      .finally(() => setLoading(false))
  }, [selectedUser, selectedChar, period])

  // Empty / placeholder states (spec §5)
  if (!users.length && history.length === 0) {
    return (
      <Page.Root>
        <Page.Header>
          <Page.Title>{t('statsTitle')}</Page.Title>
        </Page.Header>
        <p className='mt-12 text-center text-white/60'>
          {game === model.GameType.STREET_FIGHTER_6 ? t('statsEmptyTracking') : t('statsSf6Only')}
        </p>
      </Page.Root>
    )
  }

  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{t('statsTitle')}</Page.Title>
      </Page.Header>

      <div className='mb-4 flex items-center gap-2'>
        <select
          value={selectedUser}
          onChange={e => setSelectedUser(e.target.value)}
          className='h-8 min-w-[140px] rounded bg-zinc-800 px-2'
        >
          {users.map(u => (
            <option key={u.code} value={u.code}>
              {u.displayName}
            </option>
          ))}
        </select>
        <select
          value={selectedChar}
          onChange={e => setSelectedChar(e.target.value)}
          className='h-8 min-w-[120px] rounded bg-zinc-800 px-2'
        >
          {characters.map(c => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
        <select
          value={period}
          onChange={e => setPeriod(e.target.value as Period)}
          className='h-8 min-w-[120px] rounded bg-zinc-800 px-2'
        >
          <option value='7'>{t('statsPeriod7Days')}</option>
          <option value='30'>{t('statsPeriod30Days')}</option>
          <option value='all'>{t('statsPeriodAllTime')}</option>
        </select>
      </div>

      {loading && <p className='text-white/60'>{t('loading')}</p>}

      {history.length === 0 && !loading && (
        <p className='mt-8 text-center text-white/60'>{t('statsEmptyTracking')}</p>
      )}

      {history.length > 0 && (
        <>
          <div className='mb-6 grid grid-cols-3 gap-3'>
            {kpiSpecs.map(spec => {
              const curr = history[history.length - 1]
              const prev = history.length > 1 ? history[history.length - 2] : undefined
              const delta = formatDelta(
                spec.value(curr),
                prev ? spec.value(prev) : undefined,
                spec.format
              )
              return (
                <KpiCard
                  key={spec.key}
                  label={t(spec.key)}
                  value={spec.format(spec.value(curr))}
                  delta={delta}
                  tooltip={t('statsTooltip')}
                />
              )
            })}
          </div>
          <TrendChart history={history} />
          <DetailTable userId={selectedUser} character={selectedChar} />
        </>
      )}
    </Page.Root>
  )
}
