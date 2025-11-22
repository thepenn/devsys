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
