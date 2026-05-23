import React from 'react'
import { useTranslation } from 'react-i18next'

import { GetMatchesWithPlayStats } from '@cmd/CommandHandler'
import { model } from '@model'

type Props = {
  userId: string
  character: string
}

export function DetailTable({ userId, character }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = React.useState(false)
  const [rows, setRows] = React.useState<model.MatchWithStats[]>([])
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (!open) return
    let active = true
    setRows([])
    setLoading(true)
    GetMatchesWithPlayStats(userId, character, 50, 0)
      .then(r => {
        // Drop late responses for a stale (userId, character) pair so a
        // delayed previous request cannot clobber the current selection.
        if (!active) return
        setError(null)
        setRows(r ?? [])
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
              {rows.map((r, i) => (
                <tr key={`${r.match.replayId}-${i}`} className='border-t border-white/10'>
                  <td className='p-1'>{r.match.date}</td>
                  <td className='p-1'>{r.match.time}</td>
                  <td className='p-1'>{r.match.character}</td>
                  <td className='p-1'>{r.match.opponent ?? '—'}</td>
                  <td className='p-1'>{r.match.victory ? 'W' : 'L'}</td>
                  <td className='p-1 text-right'>{r.match.lpGain ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.driveImpact?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>
                    {r.stats?.receivedDriveImpact?.toFixed(1) ?? '—'}
                  </td>
                  <td className='p-1 text-right'>{r.stats?.justParry?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.throwTech?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.cornerTime?.toFixed(1) ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
