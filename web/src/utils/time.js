function toMs(value) {
  if (!value) return null
  return value > 1e12 ? value : value * 1000
}

export function formatTime(unix) {
  if (!unix) return ''
  const ts = toMs(unix)
  if (!ts) return ''
  return new Date(ts).toLocaleString()
}

export function formatDuration(start, finish) {
  if (!start) return '—'
  const startMs = toMs(start)
  const endMs = finish ? toMs(finish) : Date.now()
  const diff = Math.max(0, (endMs || Date.now()) - (startMs || Date.now()))
  const minutes = Math.floor(diff / 60000)
  const seconds = Math.floor((diff % 60000) / 1000)
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`
  }
  return `${seconds}s`
}
