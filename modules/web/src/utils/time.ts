const toMs = value => {
  if (value === null || value === undefined || value === '') return null;

  if (value instanceof Date) {
    return value.getTime();
  }

  if (typeof value === 'string') {
    const parsed = Date.parse(value);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
    const numFromString = Number(value);
    if (!Number.isNaN(numFromString)) {
      return numFromString > 1e12 ? numFromString : numFromString * 1000;
    }
    return null;
  }

  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      return null;
    }
    return value > 1e12 ? value : value * 1000;
  }

  return null;
};

export function formatTime(value) {
  const ts = toMs(value);
  if (!ts) return '';
  const date = new Date(ts);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toLocaleString();
}

export function formatDuration(start, finish) {
  if (!start) return '—';
  const startMs = toMs(start);
  const endMs = finish ? toMs(finish) : Date.now();
  if (!startMs) return '—';
  const diff = Math.max(0, (endMs || Date.now()) - startMs);
  const minutes = Math.floor(diff / 60000);
  const seconds = Math.floor((diff % 60000) / 1000);
  if (minutes > 0) {
    return `${minutes}m ${seconds.toString().padStart(2, '0')}s`;
  }
  return `${seconds}s`;
}

/** 与 formatDuration 共用：API 多为 Unix 秒，少数已为毫秒 */
function unixFieldToMs(value) {
  if (value === null || value === undefined || value === '') return null;
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) return null;
  return n > 1e12 ? n : n * 1000;
}

/**
 * 单步耗时：优先 step.finished；缺省时对终态步骤用「下一步 started」或 pipeline.finished 推断结束时间，
 * 避免第一步 finished 未落库时用 Date.now() 把 clone 拖成整条流水线时长。
 */
export function formatStepDuration(step, pipeline, orderedSteps, stepIndex) {
  if (!step) return '—';
  const startMs = unixFieldToMs(step.started);
  if (!startMs) return '—';

  const finRaw = step.finished;
  let endMs = unixFieldToMs(finRaw);

  const rawState = step.state;
  const s = String(rawState || '')
    .trim()
    .toLowerCase();
  const terminal =
    s === 'success' ||
    s === 'failure' ||
    s === 'failed' ||
    s === 'error' ||
    s === 'killed' ||
    s === 'canceled' ||
    s === 'cancelled' ||
    s === 'skipped';

  if (!endMs && terminal) {
    const next =
      Array.isArray(orderedSteps) && typeof stepIndex === 'number' && stepIndex >= 0 ? orderedSteps[stepIndex + 1] : null;
    const nextStartMs = next ? unixFieldToMs(next.started) : null;
    if (nextStartMs && nextStartMs >= startMs) {
      endMs = nextStartMs;
    } else {
      const pipeFinMs = pipeline ? unixFieldToMs(pipeline.finished) : null;
      if (pipeFinMs && pipeFinMs >= startMs) {
        endMs = pipeFinMs;
      }
    }
  }

  // 前一步 finished 若被写成整条流水线结束时间，用下一步 started 封顶，避免 clone 显示成总时长。
  const nextForCap =
    Array.isArray(orderedSteps) && typeof stepIndex === 'number' && stepIndex >= 0 ? orderedSteps[stepIndex + 1] : null;
  const nextStartCap = nextForCap ? unixFieldToMs(nextForCap.started) : null;
  if (nextStartCap && nextStartCap >= startMs && endMs && endMs > nextStartCap) {
    endMs = nextStartCap;
  }

  if (!endMs) {
    endMs = Date.now();
  }
  if (endMs < startMs) {
    endMs = startMs;
  }
  return formatDuration(startMs, endMs);
}

export function formatPodAge(timestamp) {
  if (!timestamp) return '-';
  const ts = typeof timestamp === 'number' ? timestamp * (timestamp < 1e12 ? 1000 : 1) : Number(timestamp);
  const diff = Date.now() - ts;
  if (diff <= 0) return '0s';
  const seconds = Math.floor(diff / 1000);
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) {
    return `${days}d${hours ? ` ${hours}h` : ''}`.trim();
  }
  if (hours > 0) {
    return `${hours}h${minutes ? ` ${minutes}m` : ''}`.trim();
  }
  const secs = seconds % 60;
  if (minutes > 0) {
    return `${minutes}m${secs ? ` ${secs}s` : ''}`.trim();
  }
  return `${secs}s`;
}
