export function formatRate(value: number | null | undefined): string {
  if (value == null) return '—'
  return `${(value * 100).toFixed(1)}%`
}

export function formatPerMatchCount(value: number | null | undefined): string {
  if (value == null) return '—'
  return value.toFixed(1)
}

export function formatSeconds(value: number | null | undefined): string {
  if (value == null) return '—'
  if (value < 60) return `${value}s`
  const m = Math.floor(value / 60)
  const s = value % 60
  if (m < 60) return `${m}m ${s}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export function formatDelta(
  curr: number,
  prev: number | undefined,
  fmt: (n: number) => string
): {
  text: string
  direction: 'up' | 'down' | 'flat'
} {
  if (prev === undefined) return { text: '—', direction: 'flat' }
  const diff = curr - prev
  if (Math.abs(diff) < 1e-6) return { text: '—', direction: 'flat' }
  const sign = diff > 0 ? '+' : ''
  return {
    text: `${sign}${fmt(diff)}`,
    direction: diff > 0 ? 'up' : 'down'
  }
}
