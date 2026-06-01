import React from 'react'
import { useTranslation } from 'react-i18next'
import * as Page from '@/ui/page'
import { Button } from '@/ui/button'
import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import {
  DeleteAdviceRun,
  GenerateAdviceComparison,
  GetAdviceRuns,
  GetLatestAdviceRun,
  GetPlayStatsCharacters,
  GetUsers,
  SaveAdviceFeedback
} from '@cmd/CommandHandler'
import { model } from '@model'
import { formatJSTDateTime } from '@/helpers/date'

export function AdvicePage() {
  const { t } = useTranslation()
  const trackingUser = TrackingMachineContext.useSelector(s => s.context.user)
  const game = AuthMachineContext.useSelector(s => s.context.game)
  const [users, setUsers] = React.useState<model.User[]>([])
  const [characters, setCharacters] = React.useState<string[]>([])
  const [selectedUser, setSelectedUser] = React.useState('')
  const [selectedChar, setSelectedChar] = React.useState('')
  const [run, setRun] = React.useState<model.AdviceRun | null>(null)
  const [runs, setRuns] = React.useState<model.AdviceRun[]>([])
  const [selectedMode, setSelectedMode] = React.useState('punk_record_opus_4_6')
  const [loading, setLoading] = React.useState(false)
  const [deletingRunId, setDeletingRunId] = React.useState<number | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let active = true
    GetUsers()
      .then(us => {
        if (!active) return
        setUsers(us ?? [])
        if (!selectedUser && trackingUser) setSelectedUser(trackingUser.code)
        else if (!selectedUser && us?.length) setSelectedUser(us[0].code)
      })
      .catch(e => active && setError(String(e)))
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
        if (!selectedChar && next.length) setSelectedChar(next[0])
        else if (selectedChar && !next.includes(selectedChar)) setSelectedChar(next[0] ?? '')
      })
      .catch(e => active && setError(String(e)))
    return () => {
      active = false
    }
  }, [selectedUser])

  React.useEffect(() => {
    if (!selectedUser || !selectedChar) return
    let active = true
    setLoading(true)
    Promise.all([
      GetLatestAdviceRun(selectedUser, selectedChar),
      GetAdviceRuns(selectedUser, selectedChar, 20)
    ])
      .then(([latest, history]) => {
        if (!active) return
        setRuns(history ?? [])
        setRun(latest ?? null)
        setError(null)
      })
      .catch(e => active && setError(String(e)))
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [selectedUser, selectedChar])

  const generate = React.useCallback(() => {
    if (!selectedUser || !selectedChar) return
    setLoading(true)
    GenerateAdviceComparison(selectedUser, selectedChar)
      .then(data => {
        setRun(data)
        setRuns(current => [data, ...current.filter(item => item.id !== data.id)])
        setError(null)
      })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [selectedUser, selectedChar])

  const deleteRun = React.useCallback(
    (target: model.AdviceRun) => {
      if (!window.confirm('このアドバイス履歴を削除します。よろしいですか？')) return
      setDeletingRunId(target.id)
      DeleteAdviceRun(target.id)
        .then(() => {
          setRuns(current => {
            const next = current.filter(item => item.id !== target.id)
            if (run?.id === target.id) {
              setRun(next[0] ?? null)
            }
            return next
          })
          setError(null)
        })
        .catch(e => setError(String(e)))
        .finally(() => setDeletingRunId(null))
    },
    [run?.id]
  )

  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{t('adviceTitle')}</Page.Title>
        {loading && <Page.LoadingIcon />}
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
            disabled={!selectedUser || !selectedChar || loading}
            onClick={generate}
          >
            {loading ? t('loading') : 'アドバイス生成'}
          </Button>
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

        {selectedChar && !loading && !run && (
          <p className='mt-12 text-center text-white/60'>
            まだアドバイスがありません。生成するとPunkRecordとDB Onlyの候補を比較できます。
          </p>
        )}

        {run && (
          <>
            <div className='mb-4 grid grid-cols-3 gap-3'>
              <Summary label='生成日時' value={formatJSTDateTime(run.createdAt)} />
              <Summary label='対象' value={`${run.character} / 直近${run.inputWindow}件`} />
              <Summary label='基準スナップショット' value={formatJSTDateTime(run.snapshotAt)} />
            </div>
            {runs.length > 0 && (
              <div className='mb-4 rounded border border-white/10 bg-zinc-900/35 p-3'>
                <div className='mb-2 text-xs text-white/50'>履歴</div>
                <div className='flex gap-2 overflow-x-auto pb-1'>
                  {runs.map(item => (
                    <div
                      key={item.id}
                      className={`flex min-w-[210px] items-stretch overflow-hidden rounded text-xs ${
                        item.id === run.id
                          ? 'bg-cyan-400/20 text-cyan-100'
                          : 'bg-white/8 text-white/70 hover:bg-white/12'
                      }`}
                    >
                      <button
                        type='button'
                        className='min-w-0 flex-1 px-3 py-2 text-left'
                        onClick={() => setRun(item)}
                      >
                        <div className='font-medium'>{formatJSTDateTime(item.createdAt)}</div>
                        <div className='mt-1 text-white/45'>
                          {item.character} / {item.candidates?.length ?? 0}件
                        </div>
                      </button>
                      <button
                        type='button'
                        aria-label='アドバイス履歴を削除'
                        className={`grid w-9 shrink-0 place-items-center border-l border-white/10 text-lg leading-none ${
                          item.id === run.id
                            ? 'text-cyan-100/65 hover:bg-cyan-300/15 hover:text-rose-100'
                            : 'text-white/45 hover:bg-white/12 hover:text-rose-200'
                        }`}
                        disabled={deletingRunId === item.id}
                        onClick={() => deleteRun(item)}
                      >
                        ×
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
            <AdviceCandidateTabs
              runId={run.id}
              candidates={run.candidates ?? []}
              selectedMode={selectedMode}
              onSelectMode={setSelectedMode}
            />
          </>
        )}
      </div>
    </Page.Root>
  )
}

function AdviceCandidateTabs({
  runId,
  candidates,
  selectedMode,
  onSelectMode
}: {
  runId: number
  candidates: model.AdviceCandidate[]
  selectedMode: string
  onSelectMode: (mode: string) => void
}) {
  const orderedCandidates = [...candidates].sort((a, b) => modeOrder(a.mode) - modeOrder(b.mode))
  const activeCandidate =
    orderedCandidates.find(candidate => candidate.mode === selectedMode) ?? orderedCandidates[0]

  if (!activeCandidate) return null

  return (
    <div>
      <div className='mb-3 flex border-b border-white/10'>
        {orderedCandidates.map(candidate => {
          const active = candidate.mode === activeCandidate.mode
          return (
            <button
              key={`${candidate.mode}-${candidate.id}`}
              className={`border-b-2 px-4 py-2 text-sm font-medium ${
                active
                  ? 'border-cyan-300 text-cyan-100'
                  : 'border-transparent text-white/55 hover:text-white/80'
              }`}
              onClick={() => onSelectMode(candidate.mode)}
            >
              {modeLabel(candidate.mode)}
            </button>
          )
        })}
      </div>
      <AdviceCard
        key={`${activeCandidate.mode}-${activeCandidate.id}`}
        runId={runId}
        candidate={activeCandidate}
      />
    </div>
  )
}

function modeOrder(mode: string) {
  if (mode === 'punk_record_opus_4_6') return 0
  if (mode === 'punk_record_sonnet_4_6') return 1
  if (mode === 'graph_rag') return 2
  if (mode === 'db_only') return 3
  return 4
}

function modeLabel(mode: string) {
  if (mode === 'punk_record_opus_4_6') return 'PunkRecord(Opus4.6)'
  if (mode === 'punk_record_sonnet_4_6') return 'PunkRecord(Sonnet4.6)'
  if (mode === 'graph_rag') return 'PunkRecord'
  if (mode === 'db_only') return 'DB Only'
  return mode
}

function canEvaluate(mode: string) {
  return mode !== 'db_only'
}

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded bg-zinc-800/80 p-3'>
      <div className='mb-1 text-xs text-white/50'>{label}</div>
      <div className='text-sm tabular-nums'>{value}</div>
    </div>
  )
}

function AdviceCard({ runId, candidate }: { runId: number; candidate: model.AdviceCandidate }) {
  const [saved, setSaved] = React.useState(false)
  const submitFeedback = (rating: number) => {
    SaveAdviceFeedback(runId, candidate.mode, rating, rating, rating, rating, '')
      .then(() => setSaved(true))
      .catch(() => setSaved(false))
  }

  return (
    <section className='rounded border border-white/10 bg-zinc-900/45 p-4'>
      <div className='mb-3 flex items-start justify-between gap-3'>
        <div>
          <div className='text-xs font-medium tracking-wide text-cyan-200 uppercase'>
            {modeLabel(candidate.mode)}
          </div>
          <h2 className='mt-1 text-xl font-semibold'>{candidate.theme}</h2>
        </div>
        <span className='rounded bg-white/10 px-2 py-1 text-xs text-white/70'>
          優先度 {candidate.priority}
        </span>
      </div>

      <Block title='要約' text={candidate.summary} />
      <Block title='根拠' text={candidate.rationale} />
      <Block title='施策' text={candidate.action} />
      <Block title='練習' text={candidate.drill} />
      <Block title='成功条件' text={candidate.successCriteria} />
      <Block title='監視指標' text={candidate.watchMetrics} />
      {candidate.risks && <Block title='副作用候補' text={candidate.risks} />}

      <div className='mt-4 border-t border-white/10 pt-3'>
        <div className='mb-2 text-xs text-white/50'>根拠データ</div>
        <div className='space-y-2'>
          {(candidate.evidence ?? []).map((ev, index) => (
            <div key={index} className='rounded bg-black/20 p-2 text-sm'>
              <div className='mb-1 flex items-center gap-2 text-xs text-white/45'>
                <span>{ev.source}</span>
                <span>{ev.title}</span>
                {ev.score ? <span>{ev.score.toFixed(2)}</span> : null}
              </div>
              <p className='text-white/75'>{ev.text}</p>
            </div>
          ))}
        </div>
      </div>

      {canEvaluate(candidate.mode) && (
        <div className='mt-4 flex items-center gap-2 border-t border-white/10 pt-3'>
          <span className='text-xs text-white/50'>評価</span>
          {[1, 2, 3, 4, 5].map(rating => (
            <button
              key={rating}
              className='h-8 w-8 rounded bg-white/10 text-sm hover:bg-white/20'
              onClick={() => submitFeedback(rating)}
            >
              {rating}
            </button>
          ))}
          {saved && <span className='text-xs text-emerald-300'>保存済み</span>}
        </div>
      )}
    </section>
  )
}

function Block({ title, text }: { title: string; text: string }) {
  return (
    <div className='mt-3'>
      <div className='mb-1 text-xs text-white/50'>{title}</div>
      <p className='text-sm leading-6 text-white/82'>{text || '—'}</p>
    </div>
  )
}
