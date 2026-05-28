import type { model } from '@model'

const LAST_TRACKING_USER_KEY = 'cfn-tracker:last-tracking-user'

export function rememberTrackingUser(code: string) {
  window.localStorage.setItem(LAST_TRACKING_USER_KEY, code)
}

export function lastTrackingUser(users: model.User[]) {
  const lastCode = window.localStorage.getItem(LAST_TRACKING_USER_KEY)
  return users.find(user => user.code === lastCode) ?? users[0]
}
