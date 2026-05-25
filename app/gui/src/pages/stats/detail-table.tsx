import React from 'react'
import { useTranslation } from 'react-i18next'

import { GetMatchesWithPlayStats, GetPlayStatsHistory } from '@cmd/CommandHandler'
import { model } from '@model'

type Props = {
  userId: string
  // Empty string = "all characters". Snapshots are user-wide, so the
  // character only narrows which matches we list (and therefore which
  // deltas we surface) — it never filters the snapshot stream itself.
  character: string
}

type Snapshot = model.PlayStatsSnapshot

// Capcom returns battle_stats as per-match averages over a sliding window of
// the last 100 ranked matches. The difference between two adjacent snapshots
// reflects how that average shifted across one match. Multiplying the delta
// by DELTA_SCALE (100) approximates the contribution attributable to that
// single match, since the moving average is normalized by the window size.
const DELTA_SCALE = 100

function formatDelta(value: number | undefined): string {
  if (value === undefined) return '—'
  // Round to one decimal: with DELTA_SCALE=100 most fields land on
  // whole numbers, but some (cornerTime, justParry on tiny windows)
  // benefit from the extra precision.
  const scaled = value * DELTA_SCALE
  if (Math.abs(scaled) < 0.05) return '0'
  return scaled.toFixed(1)
}

function deltaField(curr: Snapshot, prev: Snapshot | undefined, pick: (s: Snapshot) => number) {
  if (prev === undefined) return undefined
  return pick(curr) - pick(prev)
}

export function DetailTable({ userId, character }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = React.useState(false)
  const [rows, setRows] = React.useState<model.MatchWithStats[]>([])
  // User-wide snapshot history (ASC). Used to look up the snapshot that
  // immediately precedes each row's match snapshot so we can compute the
  // delta attributable to that match.
  const [snapshots, setSnapshots] = React.useState<Snapshot[]>([])
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (!open) return
    let active = true
    setRows([])
    setSnapshots([])
    setLoading(true)
    Promise.all([
      GetMatchesWithPlayStats(userId, character, 50, 0),
      // Empty character: backend ignores it (snapshots are user-wide).
      // Pull the full history so prior-snapshot lookups don't miss when a
      // match's preceding snapshot falls outside any per-character slice.
      GetPlayStatsHistory(userId, '', '', '', 0)
    ])
      .then(([matchRows, history]) => {
        // Drop late responses for a stale (userId, character) pair so a
        // delayed previous request cannot clobber the current selection.
        if (!active) return
        setError(null)
        setRows(matchRows ?? [])
        setSnapshots(history ?? [])
      })
      .catch(e => {
        if (active) setError(String(e))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      // Clear loading on cleanup so the spinner doesn't stick when the
      // detail panel collapses or props change mid-flight (the .finally
      // skips state updates once active is false).
      active = false
      setLoading(false)
    }
  }, [open, userId, character])

  // Index snapshots by id once per render so per-row lookups are O(1).
  // `snapshots` is sorted ASC by snapshot_at, so the predecessor of any
  // snapshot at index i is at index i-1 (or undefined for the very first).
  const prevById = React.useMemo(() => {
    const m = new Map<number, Snapshot>()
    for (let i = 1; i < snapshots.length; i++) {
      m.set(snapshots[i].id, snapshots[i - 1])
    }
    return m
  }, [snapshots])

  return (
    <div className='mt-6'>
      <button onClick={() => setOpen(o => !o)} className='text-sm text-white/70 hover:text-white'>
        {open ? '▼' : '▶'} {t('statsExpandDetails')}
      </button>

      {open && (
        <div className='mt-3 overflow-x-auto'>
          {loading && <p className='text-white/60'>{t('loading')}</p>}
          {error && (
            <p className='text-sm text-rose-400'>
              {t('errGetPlayStats')}: {error}
            </p>
          )}
          <table className='w-full text-xs'>
            <thead className='text-white/60'>
              <tr>
                <th className='p-1 text-left'>{t('date')}</th>
                <th className='p-1 text-left'>{t('time')}</th>
                <th className='p-1 text-left'>{t('character')}</th>
                <th className='p-1 text-left'>{t('opponent')}</th>
                <th className='p-1 text-left'>{t('result')}</th>
                <th className='p-1 text-right'>{t('tableLpDelta')}</th>
                <th className='p-1 text-right'>{t('kpiDriveImpact')}</th>
                <th className='p-1 text-right'>{t('kpiReceivedDi')}</th>
                <th className='p-1 text-right'>{t('kpiJustParry')}</th>
                <th className='p-1 text-right'>{t('kpiThrowTech')}</th>
                <th className='p-1 text-right'>{t('kpiCornerTime')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => {
                const curr = r.stats
                const prev = curr ? prevById.get(curr.id) : undefined
                const di = curr && deltaField(curr, prev, s => s.driveImpact)
                const rdi = curr && deltaField(curr, prev, s => s.receivedDriveImpact)
                const jp = curr && deltaField(curr, prev, s => s.justParry)
                const tt = curr && deltaField(curr, prev, s => s.throwTech)
                const ct = curr && deltaField(curr, prev, s => s.cornerTime)
                return (
                  <tr key={`${r.match.replayId}-${i}`} className='border-t border-white/10'>
                    <td className='p-1'>{r.match.date}</td>
                    <td className='p-1'>{r.match.time}</td>
                    <td className='p-1'>{r.match.character}</td>
                    <td className='p-1'>{r.match.opponent ?? '—'}</td>
                    <td className='p-1'>{r.match.victory ? 'W' : 'L'}</td>
                    <td className='p-1 text-right'>{r.match.lpGain ?? '—'}</td>
                    <td className='p-1 text-right'>{formatDelta(di ?? undefined)}</td>
                    <td className='p-1 text-right'>{formatDelta(rdi ?? undefined)}</td>
                    <td className='p-1 text-right'>{formatDelta(jp ?? undefined)}</td>
                    <td className='p-1 text-right'>{formatDelta(tt ?? undefined)}</td>
                    <td className='p-1 text-right'>{formatDelta(ct ?? undefined)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
