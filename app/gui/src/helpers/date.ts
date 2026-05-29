const JST_TIME_ZONE = 'Asia/Tokyo'

function parseAppDate(value: string | undefined): Date | null {
  if (!value) return null
  const trimmed = value.trim()
  if (!trimmed) return null
  const normalized = trimmed.includes('T') ? trimmed : `${trimmed.replace(' ', 'T')}Z`
  const date = new Date(normalized)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatJSTDateTime(value: string | undefined, locale = 'ja-JP') {
  const date = parseAppDate(value)
  if (!date) return value || '—'
  return new Intl.DateTimeFormat(locale, {
    timeZone: JST_TIME_ZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  }).format(date)
}

export function formatJSTTime(value: string | undefined, locale = 'ja-JP') {
  const date = parseAppDate(value)
  if (!date) return value || '—'
  return new Intl.DateTimeFormat(locale, {
    timeZone: JST_TIME_ZONE,
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }).format(date)
}

export function getJSTDay(value: string | undefined) {
  const date = parseAppDate(value)
  if (!date) return ''
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: JST_TIME_ZONE,
    day: '2-digit'
  }).format(date)
}
