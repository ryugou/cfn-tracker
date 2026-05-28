import React from 'react'
import { useTranslation } from 'react-i18next'

import * as Page from '@/ui/page'
import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import {
  GetMatchesWithPlayStats,
  GetUsers,
  GetPlayStatsCharacters,
  GetPlayStatsHistory
} from '@cmd/CommandHandler'
import { model } from '@model'
import { KpiCard } from './stats/kpi-card'
import { formatRate, formatPerMatchCount, formatSeconds, formatDelta } from './stats/formatters'
import { TrendChart } from './stats/trend-chart'
import { DetailTable } from './stats/detail-table'
import type { LocalizationKey } from '@/main/i18n'

type Period = '7' | '30' | 'all'

type Snapshot = model.PlayStatsSnapshot

const kpiSpecs: Array<{
  key: LocalizationKey
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
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let active = true
    GetUsers()
      .then(us => {
        if (!active) return
        setError(null)
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
    // Source from matches.character (per-character views are derived from
    // match deltas; snapshots are user-wide). Empty selectedChar = "all".
    GetPlayStatsCharacters(selectedUser)
      .then(cs => {
        if (!active) return
        setError(null)
        const characters = cs ?? []
        setCharacters(characters)
        // Reset to "all" when the previously selected character no longer
        // exists for this user; otherwise preserve the selection.
        if (selectedChar !== '' && !characters.includes(selectedChar)) {
          setSelectedChar('')
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
    if (!selectedUser) return
    let active = true
    setLoading(true)
    const today = new Date()
    let from = ''
    if (period === '7' || period === '30') {
      const days = period === '7' ? 7 : 30
      const d = new Date(today.getTime() - days * 86400000)
      from = d.toISOString().slice(0, 10)
    }
    const loadHistory =
      selectedChar === ''
        ? GetPlayStatsHistory(selectedUser, '', from, '', 0)
        : Promise.all([
            GetMatchesWithPlayStats(selectedUser, selectedChar, 0, 0),
            GetPlayStatsHistory(selectedUser, '', from, '', 0)
          ]).then(([matchRows, allHistory]) => {
            const matchLinked = (matchRows ?? [])
              .map(row => row.stats)
              .filter((stats): stats is model.PlayStatsSnapshot => stats !== undefined && stats !== null)
              .filter(stats => from === '' || stats.snapshotAt.slice(0, 10) >= from)

            const rows =
              matchLinked.length > 0
                ? matchLinked
                : (allHistory ?? []).filter(stats => stats.character === selectedChar)

            return rows.sort((a, b) => {
              const byTime = a.snapshotAt.localeCompare(b.snapshotAt)
              return byTime !== 0 ? byTime : a.id - b.id
            })
          })

    loadHistory
      .then(rows => {
        // Drop late responses for a stale (user, character, period) tuple so they
        // cannot overwrite the current selection's history.
        if (!active) return
        setError(null)
        setHistory(rows ?? [])
      })
      .catch(e => {
        if (active) setError(String(e))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      // Always clear loading on cleanup. The in-flight promise's `.finally`
      // skips state updates when active=false, so without this the spinner
      // would stick across rapid selection changes.
      active = false
      setLoading(false)
    }
  }, [selectedUser, selectedChar, period])

  // Empty / placeholder states (spec §5)
  if (!users.length && history.length === 0) {
    return (
      <Page.Root>
        <Page.Header>
          <Page.Title>{t('statsTitle')}</Page.Title>
        </Page.Header>
        {error && (
          <p className='mt-4 text-center text-sm text-rose-400'>
            {t('errGetPlayStats')}: {error}
          </p>
        )}
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

      <div className='min-h-0 overflow-y-scroll px-8 py-4'>
        <div className='mb-4 flex items-center gap-2'>
          <select
            value={selectedUser}
            onChange={e => setSelectedUser(e.target.value)}
            className='min-w-[140px] rounded bg-zinc-800 px-2 py-1.5'
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
            className='min-w-[120px] rounded bg-zinc-800 px-2 py-1.5'
          >
            <option value=''>{t('statsAllCharacters')}</option>
            {characters.map(c => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <select
            value={period}
            onChange={e => setPeriod(e.target.value as Period)}
            className='min-w-[120px] rounded bg-zinc-800 px-2 py-1.5'
          >
            <option value='7'>{t('statsPeriod7Days')}</option>
            <option value='30'>{t('statsPeriod30Days')}</option>
            <option value='all'>{t('statsPeriodAllTime')}</option>
          </select>
        </div>

        {selectedChar === '' && <p className='mb-4 text-xs text-white/40'>{t('statsCharScopeNote')}</p>}

        {loading && <p className='text-white/60'>{t('loading')}</p>}

        {error && (
          <p className='mt-2 text-sm text-rose-400'>
            {t('errGetPlayStats')}: {error}
          </p>
        )}

        {history.length === 0 && !loading && (
          <p className='mt-8 text-center text-white/60'>
            {game === model.GameType.STREET_FIGHTER_6 ? t('statsEmptyTracking') : t('statsSf6Only')}
          </p>
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
                  />
                )
              })}
            </div>
            <TrendChart history={history} />
            <DetailTable userId={selectedUser} character={selectedChar} />
          </>
        )}
      </div>
    </Page.Root>
  )
}
