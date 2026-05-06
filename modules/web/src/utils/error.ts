export interface HttpError extends Error {
  status: number;
}

export function normalizeError(err: unknown, fallbackMessage = '请求失败'): HttpError {
  if (!err) {
    const error = new Error(fallbackMessage) as HttpError;
    error.status = 0;
    return error;
  }
  const e = err as {
    response?: { status: number; data?: { error?: string; message?: string } };
    message?: string;
    status?: number;
  };
  if (e.response) {
    const { status, data } = e.response;
    const message = (data && (data.error || data.message)) || e.message || fallbackMessage;
    const error = new Error(message) as HttpError;
    error.status = status;
    return error;
  }
  if (typeof e.status === 'number') {
    const existing = (err instanceof Error ? err : new Error(e.message || fallbackMessage)) as HttpError;
    existing.status = e.status;
    if (!existing.message && fallbackMessage) {
      existing.message = fallbackMessage;
    }
    return existing;
  }
  const error = err instanceof Error ? (err as HttpError) : (new Error(fallbackMessage) as HttpError);
  if (typeof error.status !== 'number') {
    error.status = 0;
  }
  return error;
}
