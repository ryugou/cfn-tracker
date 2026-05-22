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

  const load = () => {
    setLoading(true)
    GetMatchesWithPlayStats(userId, character, 50, 0)
      .then(r => setRows(r ?? []))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    if (open && rows.length === 0) load()
  }, [open])

  return (
    <div className='mt-6'>
      <button
        onClick={() => setOpen(o => !o)}
        className='text-white/70 hover:text-white text-sm'
      >
        {open ? '▼' : '▶'} {t('statsExpandDetails')}
      </button>

      {open && (
        <div className='mt-3 overflow-x-auto'>
          {loading && <p className='text-white/60'>{t('loading')}</p>}
          <table className='text-xs w-full'>
            <thead className='text-white/60'>
              <tr>
                <th className='text-left p-1'>Date</th>
                <th className='text-left p-1'>Time</th>
                <th className='text-left p-1'>Char</th>
                <th className='text-left p-1'>Opp</th>
                <th className='text-left p-1'>W/L</th>
                <th className='text-right p-1'>LPΔ</th>
                <th className='text-right p-1'>DI</th>
                <th className='text-right p-1'>DI被</th>
                <th className='text-right p-1'>ジャパリ</th>
                <th className='text-right p-1'>投抜</th>
                <th className='text-right p-1'>壁秒</th>
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
                  <td className='p-1 text-right'>{r.stats?.receivedDriveImpact?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.justParry?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.throwTech?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.cornerTime ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
