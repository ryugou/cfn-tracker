import React from 'react'
import { useSelector } from '@xstate/react'
import { Outlet } from 'react-router-dom'

import { AuthMachineContext } from '@/state/auth-machine'
import { TrackingMachineContext } from '@/state/tracking-machine'

import { AppSidebar } from './app-sidebar'

export function AppWrapper() {
  const authState = AuthMachineContext.useSelector(({ value }) => value)
  const trackingState = TrackingMachineContext.useSelector(({ value }) => value)
  const showSidebar = authState === 'connected' && trackingState === 'tracking'

  return (
    <>
      {showSidebar && <AppSidebar />}
      <div className='min-w-0 flex-1'>
        <LoadingBar />
        <React.StrictMode>
          <Outlet />
        </React.StrictMode>
      </div>
    </>
  )
}

function LoadingBar() {
  const authActor = AuthMachineContext.useActorRef()
  const progress = useSelector(authActor, ({ context }) => context.progress)
  return (
    <div className='fixed top-[53px] h-1 w-full'>
      <div
        className='h-1 bg-yellow-500'
        style={{
          width: `${progress}%`,
          transition: progress > 10 ? 'width 3s ease-out' : 'width .25 ease-in'
        }}
      />
    </div>
  )
}
