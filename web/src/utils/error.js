export function normalizeError(err, fallbackMessage = '请求失败') {
  if (!err) {
    const error = new Error(fallbackMessage)
    error.status = 0
    return error
  }
  if (err.response) {
    const { status, data } = err.response
    const message = (data && (data.error || data.message)) || err.message || fallbackMessage
    const error = new Error(message)
    error.status = status
    return error
  }
  if (typeof err.status === 'number') {
    if (!err.message && fallbackMessage) {
      err.message = fallbackMessage
    }
    return err
  }
  const error = err instanceof Error ? err : new Error(fallbackMessage)
  if (typeof error.status !== 'number') {
    error.status = 0
  }
  return error
}
