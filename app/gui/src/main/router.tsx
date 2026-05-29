import { Navigate, RouterProvider as ReactRouterProvider, createHashRouter } from 'react-router-dom'

import { SettingsPage } from '@/pages/settings'
import { OutputPage } from '@/pages/output'
import { MatchesListPage } from '@/pages/matches'
import { SessionsListPage } from '@/pages/sessions'
import { StatsPage } from '@/pages/stats'
import { AnalysisPage } from '@/pages/analysis'
import { AdvicePage } from '@/pages/advice'
import { TrackingPage } from '@/pages/tracking'

import { AppWrapper } from './app-wrapper'
import { AppErrorBoundary, PageErrorBoundary } from './app-error'

import { GetUsers, GetSessions, GetMatches } from '@cmd/CommandHandler'

const router = createHashRouter([
  {
    element: <AppWrapper />,
    errorElement: <AppErrorBoundary />,
    children: [
      {
        errorElement: <PageErrorBoundary />,
        children: [
          {
            element: <Navigate to='tracking' />,
            index: true
          },
          {
            element: <TrackingPage />,
            path: '/tracking',
            loader: GetUsers
          },
          {
            element: <OutputPage />,
            path: '/output'
          },
          {
            element: <SettingsPage />,
            path: '/settings'
          },
          {
            element: <SessionsListPage />,
            path: '/sessions'
          },
          {
            element: <MatchesListPage />,
            path: '/sessions/:sessionId/matches/:page?/:limit?',
            loader: ({ params }) =>
              GetMatches(
                Number(params.sessionId),
                '',
                Number(params.page ?? 0),
                Number(params.limit ?? 0)
              )
          },
          {
            element: <StatsPage />,
            path: '/stats'
          },
          {
            element: <AnalysisPage />,
            path: '/analysis'
          },
          {
            element: <AdvicePage />,
            path: '/advice'
          }
        ]
      }
    ]
  }
])

export const RouterProvider = () => <ReactRouterProvider router={router} />
