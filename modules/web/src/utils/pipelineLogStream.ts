import { getToken } from './auth';
import { API_BASE_URL } from './request';

/** Aligns with model.LogEntryType (Go). */
export function pipelineLogTypeToString(t: number | string | undefined): string {
  if (typeof t === 'string') return t;
  switch (t) {
    case 0:
      return 'stdout';
    case 1:
      return 'stderr';
    case 2:
      return 'exit_code';
    case 3:
      return 'metadata';
    case 4:
      return 'progress';
    default:
      return 'unknown';
  }
}

export type WoodpeckerLogLine = {
  line: number;
  type?: number | string;
  time?: number;
  content?: string;
  out?: string;
};

type SSEFrame = {
  id?: string;
  event?: string;
  data: string;
};

function extractCompleteSSEFrames(buffer: string): { frames: SSEFrame[]; rest: string } {
  const frames: SSEFrame[] = [];
  let rest = buffer;
  for (;;) {
    const sep = rest.indexOf('\n\n');
    if (sep === -1) break;
    const raw = rest.slice(0, sep);
    rest = rest.slice(sep + 2);
    let id: string | undefined;
    let event: string | undefined;
    const dataLines: string[] = [];
    for (const line of raw.split('\n')) {
      if (line.startsWith(':')) continue;
      if (line.startsWith('id:')) {
        id = line.slice(3).trim();
        continue;
      }
      if (line.startsWith('event:')) {
        event = line.slice(6).trim();
        continue;
      }
      if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).trimStart());
      }
    }
    const data = dataLines.join('\n');
    if (data !== '' || event) {
      frames.push({ id, event, data });
    }
  }
  return { frames, rest };
}

function resolveUrl(path: string): string {
  return path.startsWith('http') ? path : `${API_BASE_URL.replace(/\/+$/, '')}${path.startsWith('/') ? path : `/${path}`}`;
}

/**
 * Woodpecker-compatible SSE log stream with Bearer auth.
 * Sends Last-Event-ID on reconnect after a dropped connection.
 */
export async function consumePipelineLogSSE(
  path: string,
  handlers: {
    onLogLine?: (row: WoodpeckerLogLine) => void;
    /** Terminal stream error from server (event: error). */
    onStreamErrorMessage?: (msg: string) => void;
    onError?: (err: Error) => void;
  },
  signal: AbortSignal
): Promise<void> {
  const url = resolveUrl(path);
  let lastEventId = 0;
  const maxReconnectAttempts = 5;

  for (let attempt = 0; !signal.aborted && attempt < maxReconnectAttempts; attempt++) {
    if (attempt > 0) {
      await new Promise<void>(resolve => {
        const t = window.setTimeout(resolve, 800);
        signal.addEventListener('abort', () => window.clearTimeout(t), { once: true });
      });
      if (signal.aborted) return;
    }

    const headers: Record<string, string> = { Accept: 'text/event-stream' };
    const token = getToken();
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    if (lastEventId > 0) {
      headers['Last-Event-ID'] = String(lastEventId);
    }

    let response: Response;
    try {
      response = await fetch(url, { method: 'GET', headers, signal });
    } catch (e) {
      if (signal.aborted) return;
      if (attempt === maxReconnectAttempts - 1) {
        handlers.onError?.(e instanceof Error ? e : new Error(String(e)));
      }
      continue;
    }

    if (!response.ok) {
      const text = await response.text().catch(() => '');
      handlers.onError?.(new Error(text || `HTTP ${response.status}`));
      return;
    }

    const reader = response.body?.getReader();
    if (!reader) {
      handlers.onError?.(new Error('no response body'));
      return;
    }

    const decoder = new TextDecoder();
    let carry = '';
    let sawEOF = false;
    let sawStreamError = false;

    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        carry += decoder.decode(value, { stream: true });
        const { frames, rest } = extractCompleteSSEFrames(carry);
        carry = rest;
        for (const fr of frames) {
          const ev = (fr.event || '').trim();
          if (ev === 'error') {
            handlers.onStreamErrorMessage?.(fr.data || 'error');
            sawStreamError = true;
            return;
          }
          if (ev === 'eof') {
            sawEOF = true;
            return;
          }
          if (!fr.data) continue;
          try {
            const obj = JSON.parse(fr.data) as Record<string, unknown>;
            if (typeof obj.line === 'number') {
              if (fr.id && /^\d+$/.test(fr.id)) {
                lastEventId = parseInt(fr.id, 10);
              }
              handlers.onLogLine?.({
                line: obj.line,
                type: (obj.type as number | string) ?? 0,
                time: typeof obj.time === 'number' ? obj.time : undefined,
                content: typeof obj.content === 'string' ? obj.content : undefined,
                out: typeof obj.out === 'string' ? obj.out : undefined
              });
            }
          } catch {
            // ignore non-JSON data lines (e.g. eof payload if mis-parsed)
          }
        }
      }
    } catch (e) {
      if (signal.aborted) return;
      if (attempt === maxReconnectAttempts - 1) {
        handlers.onError?.(e instanceof Error ? e : new Error(String(e)));
      }
      continue;
    }

    if (sawEOF || sawStreamError) {
      return;
    }
    if (signal.aborted) return;
    if (lastEventId > 0) {
      continue;
    }
    return;
  }
}
