import React from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from 'recharts'

import * as Page from '@/ui/page'
import { Button } from '@/ui/button'
import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import {
  GetBenchmarkComparison,
  GetPlayStatsCharacters,
  GetUsers,
  RefreshBenchmarkPlayers
} from '@cmd/CommandHandler'
import { EventsOff, EventsOn } from '@runtime'
import { model } from '@model'

import { formatPerMatchCount, formatRate, formatSeconds } from './stats/formatters'
import type { LocalizationKey } from '@/main/i18n'

type Metric = {
  key: keyof model.PlayStatsSnapshot
  labelKey: LocalizationKey
  format: (value: number | null | undefined) => string
  higherIsBetter?: boolean
}

const metrics: Metric[] = [
  { key: 'driveImpact', labelKey: 'kpiDriveImpact', format: formatPerMatchCount, higherIsBetter: true },
  { key: 'receivedDriveImpact', labelKey: 'kpiReceivedDi', format: formatPerMatchCount },
  { key: 'justParry', labelKey: 'kpiJustParry', format: formatPerMatchCount, higherIsBetter: true },
  { key: 'throwTech', labelKey: 'kpiThrowTech', format: formatPerMatchCount, higherIsBetter: true },
  { key: 'cornerTime', labelKey: 'kpiCornerTime', format: formatSeconds, higherIsBetter: true },
  { key: 'corneredTime', labelKey: 'kpiCorneredTime', format: formatSeconds },
  { key: 'gaugeRateDriveImpact', labelKey: 'kpiGaugeDi', format: formatRate, higherIsBetter: true },
  { key: 'gaugeRateDriveRushFromCancel', labelKey: 'kpiGaugeDrc', format: formatRate, higherIsBetter: true },
  { key: 'gaugeRateSALv3', labelKey: 'kpiSaLv3', format: formatRate, higherIsBetter: true },
  { key: 'punishCounter', labelKey: 'kpiPunishCounter', format: formatPerMatchCount, higherIsBetter: true },
  { key: 'receivedPunishCounter', labelKey: 'kpiReceivedPunishCounter', format: formatPerMatchCount },
  { key: 'throwCount', labelKey: 'kpiThrowCount', format: formatPerMatchCount, higherIsBetter: true },
  { key: 'receivedThrowCount', labelKey: 'kpiReceivedThrowCount', format: formatPerMatchCount }
]

function numeric(stats: model.PlayStatsSnapshot | undefined, key: keyof model.PlayStatsSnapshot) {
  const value = stats?.[key]
  return typeof value === 'number' ? value : undefined
}

function normalizeComparison(data: model.BenchmarkComparison | null | undefined): model.BenchmarkComparison {
  return {
    self: data?.self,
    players: data?.players ?? [],
    rankAverages: data?.rankAverages ?? []
  }
}

function deltaClass(metric: Metric, value: number | undefined) {
  if (value == null || Math.abs(value) < 1e-6) return 'text-white/45'
  const good = metric.higherIsBetter ? value > 0 : value < 0
  return good ? 'text-emerald-400' : 'text-rose-400'
}

function signed(value: number | undefined, format: Metric['format']) {
  if (value == null) return '—'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${format(value)}`
}

function benchmarkGroupLabel(
  t: TFunction,
  players: model.BenchmarkPlayer[],
  rankOffset: number
) {
  const isMaster = players.some(player => player.rankOffset === rankOffset && player.leagueRank >= 36)
  if (isMaster) {
    return t(rankOffset === 1 ? 'analysisMaster100' : 'analysisMaster200')
  }
  return t(rankOffset === 1 ? 'analysisRank1' : 'analysisRank2')
}

const chartTick = { fill: 'rgba(255,255,255,.78)', fontSize: 11 }
const chartLegend = { color: 'rgba(255,255,255,.86)', fontSize: 12 }
const chartTooltip = {
  background: '#1f1f23',
  border: '1px solid rgba(255,255,255,.18)',
  color: '#f4f4f5'
}

type BenchmarkRefreshProgress = {
  userId: string
  character: string
  completed: number
  total: number
}

export function AnalysisPage() {
  const { t } = useTranslation()
  const trackingUser = TrackingMachineContext.useSelector(s => s.context.user)
  const game = AuthMachineContext.useSelector(s => s.context.game)

  const [users, setUsers] = React.useState<model.User[]>([])
  const [characters, setCharacters] = React.useState<string[]>([])
  const [selectedUser, setSelectedUser] = React.useState('')
  const [selectedChar, setSelectedChar] = React.useState('')
  const [comparison, setComparison] = React.useState<model.BenchmarkComparison | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [refreshing, setRefreshing] = React.useState(false)
  const [refreshProgress, setRefreshProgress] = React.useState<BenchmarkRefreshProgress | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let active = true
    GetUsers()
      .then(us => {
        if (!active) return
        setUsers(us ?? [])
        if (!selectedUser && trackingUser) {
          setSelectedUser(trackingUser.code)
        } else if (!selectedUser && us?.length) {
          setSelectedUser(us[0].code)
        }
      })
      .catch(e => {
        if (active) setError(String(e))
      })
    return () => {
      active = false
    }
  }, [trackingUser])

  React.useEffect(() => {
    if (!selectedUser) return
    let active = true
    GetPlayStatsCharacters(selectedUser)
      .then(cs => {
        if (!active) return
        const next = cs ?? []
        setCharacters(next)
        if (!selectedChar && next.length) {
          setSelectedChar(next[0])
        } else if (selectedChar && !next.includes(selectedChar)) {
          setSelectedChar(next[0] ?? '')
        }
      })
      .catch(e => {
        if (active) setError(String(e))
      })
    return () => {
      active = false
    }
  }, [selectedUser])

  React.useEffect(() => {
    if (!selectedUser || !selectedChar) return
    let active = true
    setLoading(true)
    GetBenchmarkComparison(selectedUser, selectedChar)
      .then(data => {
        if (!active) return
        setError(null)
        setComparison(normalizeComparison(data))
      })
      .catch(e => {
        if (active) setError(String(e))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [selectedUser, selectedChar])

  React.useEffect(() => {
    EventsOn('benchmark-refresh-progress', (progress: BenchmarkRefreshProgress) => {
      setRefreshProgress(current => {
        if (progress.userId !== selectedUser || progress.character !== selectedChar) return current
        return progress
      })
    })
    return () => {
      EventsOff('benchmark-refresh-progress')
    }
  }, [selectedUser, selectedChar])

  const refresh = React.useCallback(() => {
    if (!selectedUser || !selectedChar) return
    setRefreshing(true)
    setRefreshProgress({ userId: selectedUser, character: selectedChar, completed: 0, total: 10 })
    RefreshBenchmarkPlayers(selectedUser, selectedChar)
      .then(() => GetBenchmarkComparison(selectedUser, selectedChar))
      .then(data => {
        setError(null)
        setComparison(normalizeComparison(data))
      })
      .catch(e => setError(String(e)))
      .finally(() => {
        setRefreshing(false)
        setRefreshProgress(null)
      })
  }, [selectedUser, selectedChar])

  const averages = React.useMemo(() => {
    const out = new Map<number, model.BenchmarkRankAverage>()
    comparison?.rankAverages?.forEach(avg => out.set(avg.rankOffset, avg))
    return out
  }, [comparison])
  const players = comparison?.players ?? []
  const rank1Label = benchmarkGroupLabel(t, players, 1)
  const rank2Label = benchmarkGroupLabel(t, players, 2)

  const chartData = metrics.map(metric => ({
    metric: t(metric.labelKey),
    self: numeric(comparison?.self, metric.key),
    rank1: numeric(averages.get(1)?.stats, metric.key),
    rank2: numeric(averages.get(2)?.stats, metric.key)
  }))

  const playerChartData = metrics.map(metric => {
    const values = players
      ?.filter(p => p.stats)
      .map(p => numeric(p.stats, metric.key))
      .filter((value): value is number => value !== undefined)
    const avg = values?.length ? values.reduce((sum, value) => sum + value, 0) / values.length : undefined
    return {
      metric: t(metric.labelKey),
      self: numeric(comparison?.self, metric.key),
      field: avg
    }
  })

  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{t('analysisTitle')}</Page.Title>
        {(loading || refreshing) && <Page.LoadingIcon />}
      </Page.Header>

      <div className='min-h-0 overflow-y-scroll px-8 py-4'>
        <div className='mb-4 flex flex-wrap items-center gap-2'>
          <select
            value={selectedUser}
            onChange={e => setSelectedUser(e.target.value)}
            className='min-w-[150px] rounded bg-zinc-800 px-2 py-1.5'
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
            className='min-w-[130px] rounded bg-zinc-800 px-2 py-1.5'
          >
            {characters.map(c => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <Button
            className='rounded px-4 py-1.5 text-sm'
            disabled={!selectedUser || !selectedChar || refreshing}
            onClick={refresh}
          >
            {refreshing ? t('loading') : t('analysisRefresh')}
          </Button>
          {refreshing && refreshProgress && (
            <span className='text-sm tabular-nums text-white/70'>
              {refreshProgress.completed}/{refreshProgress.total}
            </span>
          )}
        </div>

        {error && (
          <p className='mb-4 text-sm text-rose-400'>
            {t('errGetPlayStats')}: {error}
          </p>
        )}

        {!selectedChar && (
          <p className='mt-12 text-center text-white/60'>
            {game === model.GameType.STREET_FIGHTER_6 ? t('statsEmptyTracking') : t('statsSf6Only')}
          </p>
        )}

        {selectedChar && !loading && players.length === 0 && (
          <p className='mt-12 text-center text-white/60'>{t('analysisEmpty')}</p>
        )}

        {comparison && players.length > 0 && (
          <>
            <div className='mb-4 grid grid-cols-3 gap-3'>
              <Summary label={t('analysisSelf')} value={comparison.self?.snapshotAt ?? '—'} />
              <Summary
                label={rank1Label}
                value={`${averages.get(1)?.count ?? 0} / ${players.filter(p => p.rankOffset === 1).length}`}
              />
              <Summary
                label={rank2Label}
                value={`${averages.get(2)?.count ?? 0} / ${players.filter(p => p.rankOffset === 2).length}`}
              />
            </div>

            <div className='mb-6 h-[30rem]'>
              <ResponsiveContainer>
                <BarChart data={chartData} margin={{ top: 18, right: 20, left: 0, bottom: 108 }}>
                  <CartesianGrid stroke='rgba(255,255,255,.08)' />
                  <XAxis
                    dataKey='metric'
                    angle={-28}
                    textAnchor='end'
                    interval={0}
                    height={106}
                    tick={chartTick}
                    tickMargin={8}
                    stroke='rgba(255,255,255,.3)'
                  />
                  <YAxis tick={chartTick} stroke='rgba(255,255,255,.3)' />
                  <Tooltip contentStyle={chartTooltip} labelStyle={{ color: '#f4f4f5' }} />
                  <Legend verticalAlign='top' align='center' wrapperStyle={chartLegend} />
                  <Bar dataKey='self' name={t('analysisSelf')} fill='#60a5fa' />
                  <Bar dataKey='rank1' name={rank1Label} fill='#34d399' />
                  <Bar dataKey='rank2' name={rank2Label} fill='#fbbf24' />
                </BarChart>
              </ResponsiveContainer>
            </div>

            <div className='mb-6 h-[28rem]'>
              <ResponsiveContainer>
                <BarChart data={playerChartData} margin={{ top: 18, right: 20, left: 0, bottom: 104 }}>
                  <CartesianGrid stroke='rgba(255,255,255,.08)' />
                  <XAxis
                    dataKey='metric'
                    angle={-25}
                    textAnchor='end'
                    interval={0}
                    height={102}
                    tick={chartTick}
                    tickMargin={8}
                    stroke='rgba(255,255,255,.3)'
                  />
                  <YAxis tick={chartTick} stroke='rgba(255,255,255,.3)' />
                  <Tooltip contentStyle={chartTooltip} labelStyle={{ color: '#f4f4f5' }} />
                  <Legend verticalAlign='top' align='center' wrapperStyle={chartLegend} />
                  <Bar dataKey='self' name={t('analysisSelf')} fill='#60a5fa' />
                  <Bar dataKey='field' name={t('analysisBenchmarkAverage')} fill='#a78bfa' />
                </BarChart>
              </ResponsiveContainer>
            </div>

            <ComparisonTable
              self={comparison.self}
              rank1={averages.get(1)?.stats}
              rank2={averages.get(2)?.stats}
              rank1Label={rank1Label}
              rank2Label={rank2Label}
            />
            <PlayersTable players={players} />
          </>
        )}
      </div>
    </Page.Root>
  )
}

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded bg-zinc-800/80 p-3'>
      <div className='mb-1 text-xs text-white/50'>{label}</div>
      <div className='text-lg tabular-nums'>{value}</div>
    </div>
  )
}

function ComparisonTable({
  self,
  rank1,
  rank2,
  rank1Label,
  rank2Label
}: {
  self?: model.PlayStatsSnapshot
  rank1?: model.PlayStatsSnapshot
  rank2?: model.PlayStatsSnapshot
  rank1Label: string
  rank2Label: string
}) {
  const { t } = useTranslation()
  return (
    <div className='mb-6 overflow-hidden rounded border border-white/10'>
      <table className='w-full text-sm'>
        <thead className='bg-zinc-900/80 text-left text-xs text-white/60'>
          <tr>
            <th className='px-3 py-2'>{t('analysisMetric')}</th>
            <th className='px-3 py-2'>{t('analysisSelf')}</th>
            <th className='px-3 py-2'>{rank1Label}</th>
            <th className='px-3 py-2'>{rank2Label}</th>
            <th className='px-3 py-2'>{t('analysisDeltaTo', { target: rank1Label })}</th>
            <th className='px-3 py-2'>{t('analysisDeltaTo', { target: rank2Label })}</th>
          </tr>
        </thead>
        <tbody>
          {metrics.map(metric => {
            const selfValue = numeric(self, metric.key)
            const rank1Value = numeric(rank1, metric.key)
            const rank2Value = numeric(rank2, metric.key)
            const delta1 =
              selfValue !== undefined && rank1Value !== undefined ? selfValue - rank1Value : undefined
            const delta2 =
              selfValue !== undefined && rank2Value !== undefined ? selfValue - rank2Value : undefined
            return (
              <tr key={metric.key} className='border-t border-white/10 odd:bg-white/[0.02]'>
                <td className='px-3 py-2 text-white/80'>{t(metric.labelKey)}</td>
                <td className='px-3 py-2 tabular-nums'>{metric.format(selfValue)}</td>
                <td className='px-3 py-2 tabular-nums'>{metric.format(rank1Value)}</td>
                <td className='px-3 py-2 tabular-nums'>{metric.format(rank2Value)}</td>
                <td className={`px-3 py-2 tabular-nums ${deltaClass(metric, delta1)}`}>
                  {signed(delta1, metric.format)}
                </td>
                <td className={`px-3 py-2 tabular-nums ${deltaClass(metric, delta2)}`}>
                  {signed(delta2, metric.format)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function PlayersTable({ players }: { players: model.BenchmarkPlayer[] }) {
  const { t } = useTranslation()
  return (
    <div className='overflow-x-auto rounded border border-white/10'>
      <table className='min-w-max text-sm'>
        <thead className='bg-zinc-900/80 text-left text-xs text-white/60'>
          <tr>
            <th className='sticky left-0 z-10 min-w-[150px] bg-zinc-900 px-3 py-2'>{t('user')}</th>
            <th className='min-w-[92px] px-3 py-2'>{t('analysisGroup')}</th>
            <th className='min-w-[70px] px-3 py-2 text-right'>LP</th>
            <th className='min-w-[70px] px-3 py-2 text-right'>MR</th>
            {metrics.map(metric => (
              <th key={metric.key} className='min-w-[112px] px-3 py-2 text-right'>
                {t(metric.labelKey)}
              </th>
            ))}
            <th className='min-w-[140px] px-3 py-2'>{t('analysisFetchedAt')}</th>
          </tr>
        </thead>
        <tbody>
          {players.map(player => (
            <tr key={`${player.rankOffset}-${player.targetUserId}`} className='border-t border-white/10 odd:bg-white/[0.02]'>
              <td className='sticky left-0 bg-[#111827] px-3 py-2'>
                <div className='font-medium'>{player.fighterId}</div>
                <div className='text-xs text-white/40'>{player.targetUserId}</div>
                {player.lastError && <div className='mt-1 text-xs text-rose-400'>{player.lastError}</div>}
              </td>
              <td className='px-3 py-2'>
                {benchmarkGroupLabel(t, players, player.rankOffset)}
              </td>
              <td className='px-3 py-2 text-right tabular-nums'>{player.lp}</td>
              <td className='px-3 py-2 text-right tabular-nums'>{player.mr || '—'}</td>
              {metrics.map(metric => (
                <td key={metric.key} className='px-3 py-2 text-right tabular-nums'>
                  {metric.format(numeric(player.stats, metric.key))}
                </td>
              ))}
              <td className='px-3 py-2 text-xs text-white/50'>{player.fetchedAt || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
