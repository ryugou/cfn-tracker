import React from 'react'
import { motion } from 'framer-motion'
import { Icon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { useSelector } from '@xstate/react'
import { PieChart } from 'react-minimal-pie-chart'

import { TrackingMachineContext } from '@/state/tracking-machine'
import { Button } from '@/ui/button'
import { Tooltip } from '@/ui/tooltip'
import * as Page from '@/ui/page'
import { type LocalizationKey } from '@/main/i18n'
import { GetPlayStatsCharacters, GetLatestMatchForUserAndCharacter } from '@cmd/CommandHandler'
import type { model } from '@model'

export function TrackingLiveUpdater() {
  const { t } = useTranslation()
  const trackingActor = TrackingMachineContext.useActorRef()

  const liveMatch = useSelector(trackingActor, ({ context: { match } }) => match)
  const userCode = useSelector(trackingActor, ({ context: { user } }) => user?.code ?? '')

  // Character selector state. Empty string = "not yet chosen"; on first load
  // we auto-select the character from the latest live match so the screen
  // behaves identically to before until the user picks something else.
  const [characters, setCharacters] = React.useState<string[]>([])
  const [selectedChar, setSelectedChar] = React.useState<string>('')
  // The match actually rendered. We keep this as standalone state (rather
  // than deriving it on every render) so that live events for *non*-selected
  // characters can be silently ignored without flicker, and so that a failed
  // DB fetch leaves the previous display intact instead of falling back to
  // the wrong-character live match.
  const [displayMatch, setDisplayMatch] = React.useState<model.Match>(liveMatch)
  const userInteracted = React.useRef(false)

  // Pull the distinct characters the user has played from the matches table.
  // Re-fetch when a live match arrives so a freshly-played character shows up
  // in the dropdown without forcing the user to restart tracking.
  React.useEffect(() => {
    if (!userCode) return
    let active = true
    GetPlayStatsCharacters(userCode)
      .then(cs => {
        if (!active) return
        const list = cs ?? []
        // Make sure the currently live character is selectable even if the
        // DB hasn't seen it yet (e.g. very first match of a new character).
        if (liveMatch.character && !list.includes(liveMatch.character)) {
          list.push(liveMatch.character)
          list.sort()
        }
        setCharacters(list)
      })
      .catch(() => {
        /* selector is best-effort; failures fall back to live-only data */
      })
    return () => {
      active = false
    }
  }, [userCode, liveMatch.character])

  // Auto-select the live character on first paint. We only do this when the
  // user hasn't manually picked one yet — otherwise their choice would get
  // clobbered every time a new match arrives.
  React.useEffect(() => {
    if (userInteracted.current) return
    if (!liveMatch.character) return
    setSelectedChar(prev => (prev === '' ? liveMatch.character : prev))
  }, [liveMatch.character])

  // Apply live match events to the display only when they match the selected
  // character. Once a selection exists, events for other characters are
  // intentionally ignored so the user's view doesn't flicker. Before the
  // user has chosen anything (selectedChar === ''), accept any live match —
  // this preserves the pre-selector behavior for the very first paint.
  React.useEffect(() => {
    if (!liveMatch.character) return
    if (selectedChar === '' || liveMatch.character === selectedChar) {
      setDisplayMatch(liveMatch)
    }
  }, [liveMatch, selectedChar])

  // When the user picks a character that isn't the live one, fetch the most
  // recent stored match for that character and overwrite the display. If the
  // fetch fails or returns nothing, leave the previous display in place
  // rather than falling back to a wrong-character live match.
  React.useEffect(() => {
    if (!userCode || !selectedChar) return
    if (selectedChar === liveMatch.character) {
      // Live already covers the selection — no DB hit needed; the live-match
      // effect above will keep displayMatch in sync.
      return
    }
    let active = true
    GetLatestMatchForUserAndCharacter(userCode, selectedChar)
      .then(m => {
        if (!active) return
        if (m && m.character) setDisplayMatch(m)
        // No match in DB for this character: keep current display.
      })
      .catch(() => {
        /* preserve previous displayMatch on failure */
      })
    return () => {
      active = false
    }
  }, [userCode, selectedChar, liveMatch.character])

  const {
    lp,
    mr,
    wins,
    losses,
    winStreak,
    lpGain,
    mrGain,
    winRate,
    opponent,
    opponentCharacter,
    character,
    opponentLeague,
    userName,
    victory
  } = displayMatch

  const [refreshDisabled, setRefreshDisabled] = React.useState(false)

  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{t('tracking')}</Page.Title>
        <Page.LoadingIcon />
      </Page.Header>
      <motion.section
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.125 }}
        className='h-full px-6 pt-4'
      >
        <dl className='flex w-full items-center justify-between whitespace-nowrap'>
          <SmallStat text='CFN' value={userName} />
          <div className='flex justify-between gap-8'>
            {lp != 0 && <SmallStat text='LP' value={`${lp == -1 ? t('placement') : lp}`} />}
            {mr != 0 && <SmallStat text='MR' value={`${mr == -1 ? t('placement') : mr}`} />}
          </div>
        </dl>
        {characters.length > 0 && (
          <div className='mt-2 flex items-center gap-2 text-sm'>
            <label htmlFor='tracking-character' className='text-white/70'>
              {t('statsCharacter')}
            </label>
            <select
              id='tracking-character'
              value={selectedChar}
              onChange={e => {
                userInteracted.current = true
                setSelectedChar(e.target.value)
              }}
              className='h-7 min-w-[120px] rounded bg-zinc-800 px-2'
            >
              {characters.map(c => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
        )}
        <div className='flex h-[calc(100%-32px)] flex-1 pt-3 pb-5'>
          <div className='w-full'>
            <dl className='text-lg whitespace-nowrap'>
              <div className='flex justify-between gap-2'>
                <BigStat text={t('wins')} value={wins} />
                <BigStat text={t('losses')} value={losses} />
              </div>
              <div className='flex justify-between gap-2'>
                <BigStat text={t('winRate')} value={`${winRate}%`} />
                <BigStat text={t('winStreak')} value={winStreak} />
              </div>
              <div className='flex justify-between gap-2'>
                {lpGain != 0 && (
                  <BigStat text={t('lpGain')} value={`${lpGain > 0 ? `+` : ``}${lpGain}`} />
                )}
                {mrGain != 0 && (
                  <BigStat text={t('mrGain')} value={`${mrGain > 0 ? `+` : ``}${mrGain}`} />
                )}
              </div>
            </dl>
            {opponent != '' && (
              <div className='group flex items-center justify-between rounded-xl bg-slate-50/5 p-3 pb-2 text-lg leading-none'>
                <span>{t('lastMatch')}</span>
                <div className='relative flex items-center gap-2'>
                  <Icon
                    icon={victory ? 'twemoji:victory-hand' : 'twemoji:pensive-face'}
                    width={25}
                  />{' '}
                  vs
                  <b>{opponent}</b> - {opponentCharacter} ({t(opponentLeague as LocalizationKey)})
                </div>
              </div>
            )}
          </div>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.35 }}
            className='relative grid h-full flex-0'
          >
            <PieChart
              className='mx-auto h-52 w-full'
              animate
              animationDuration={2000}
              lineWidth={85}
              data={[
                {
                  title: t('wins'),
                  value: wins,
                  color: 'rgba(0, 255, 77, .65)'
                },
                {
                  title: t('losses'),
                  value: wins == 0 && losses == 0 ? 1 : losses,
                  color: 'rgba(251, 73, 73, 0.25)'
                }
              ]}
            >
              <defs>
                <linearGradient id='blue-gradient' direction={-65}>
                  <stop offset='0%' stopColor='#20BF55' />
                  <stop offset='100%' stopColor='#347fd0' />
                </linearGradient>
                <linearGradient id='red-gradient' direction={120}>
                  <stop offset='0%' stopColor='#EC9F05' />
                  <stop offset='100%' stopColor='#EE9617' />
                </linearGradient>
              </defs>
            </PieChart>
            <div className='flex justify-between gap-2 self-end'>
              <Tooltip text={t('cooldown')} disabled={!refreshDisabled}>
                <Button
                  style={{ filter: 'hue-rotate(-160deg)' }}
                  disabled={refreshDisabled}
                  onClick={() => {
                    if (!refreshDisabled) {
                      trackingActor.send({ type: 'forcePoll' })
                      setRefreshDisabled(true)
                      setTimeout(() => setRefreshDisabled(false), 15000)
                    }
                  }}
                >
                  <Icon icon='fa6-solid:recycle' className='mr-3 h-5 w-5' />
                  {t('refresh')}
                </Button>
              </Tooltip>
              <Button onClick={() => trackingActor.send({ type: 'cease' })}>
                <Icon icon='fa6-solid:stop' className='mr-3 h-5 w-5' />
                {t('stop')}
              </Button>
            </div>
          </motion.div>
        </div>
        {/* TODO: fix character image for tekken 8 */}
        {character && (
          <img
            className='pointer-events-none absolute top-0 -right-20 z-[-1] h-full opacity-10 grayscale'
            src={`https://www.streetfighter.com/6/buckler/assets/images/material/character/character_${character
              .toLowerCase()
              .replace(/\s/g, '')
              .replace('.', '')}_r.png`}
            alt={''}
          />
        )}
      </motion.section>
    </Page.Root>
  )
}

type StatProps = { text: string; value: string | number }
const BigStat = ({ text, value }: StatProps) => (
  <div className='mb-2 flex flex-1 justify-between gap-4 rounded-xl bg-slate-50/5 p-3 pb-1'>
    <dt className='font-extralight tracking-wider'>{text}</dt>
    <dd className='text-4xl font-semibold'>{value}</dd>
  </div>
)

const SmallStat = ({ text, value }: StatProps) => (
  <div className='flex gap-3 text-2xl'>
    <dt className='text-xl leading-8'>{text}</dt>
    <dd className='font-bold'>{value}</dd>
  </div>
)
