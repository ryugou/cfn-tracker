import React from 'react'
import { useLoaderData } from 'react-router-dom'
import { useSelector } from '@xstate/react'
import { useTranslation } from 'react-i18next'

import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import { useErrorPopup } from '@/main/error-popup'
import * as Page from '@/ui/page'
import { model } from '@model'

import { TrackingForm } from './tracking-form'
import { TrackingGamePicker } from './tracking-game-picker'
import { TrackingLiveUpdater } from './tracking-live-updater'
import { lastTrackingUser, rememberTrackingUser } from './preferences'

export function TrackingPage() {
  const { t } = useTranslation()

  const trackingActor = TrackingMachineContext.useActorRef()
  const authActor = AuthMachineContext.useActorRef()

  const users = (useLoaderData() ?? []) as model.User[]
  const authState = useSelector(authActor, ({ value }) => value)
  const trackingState = useSelector(trackingActor, ({ value }) => value)

  const authError = useSelector(authActor, ({ context }) => context.error)
  const trackingError = useSelector(trackingActor, ({ context }) => context.error)

  const setError = useErrorPopup()
  const autoStarted = React.useRef(false)

  React.useEffect(() => {
    authError && setError(authError)
  }, [authError])

  React.useEffect(() => {
    trackingError && setError(trackingError)
  }, [trackingError])

  switch (authState) {
    case 'gameForm':
      return <TrackingGamePicker onSubmit={game => authActor.send({ type: 'submit', game })} />
    case 'loading':
      return (
        <LoadingTracking title={t('loading')} />
      )
  }

  switch (trackingState) {
    case 'cfnForm':
      if (users.length > 0 && !autoStarted.current) return <AutoStartTracking users={users} autoStarted={autoStarted} />
      return <TrackingForm />
    case 'tracking':
      return <TrackingLiveUpdater />
    case 'loading':
    default:
      return <LoadingTracking title={t('loading')} />
  }
}

function AutoStartTracking({
  users,
  autoStarted
}: {
  users: model.User[]
  autoStarted: React.MutableRefObject<boolean>
}) {
  const { t } = useTranslation()
  const trackingActor = TrackingMachineContext.useActorRef()

  React.useEffect(() => {
    if (autoStarted.current) return
    autoStarted.current = true
    const user = lastTrackingUser(users)
    rememberTrackingUser(user.code)
    trackingActor.send({ type: 'submit', user, restore: true })
  }, [autoStarted, trackingActor, users])

  return <LoadingTracking title={t('loading')} />
}

function LoadingTracking({ title }: { title: string }) {
  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{title}</Page.Title>
        <Page.LoadingIcon />
      </Page.Header>
    </Page.Root>
  )
}
