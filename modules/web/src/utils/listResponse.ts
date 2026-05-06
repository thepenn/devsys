/** Normalizes paginated `{ items, total, ... }` or bare array API responses. */
export function itemsFromListResponse<T = any>(data: unknown): T[] {
  if (Array.isArray(data)) {
    return data as T[];
  }
  if (data && typeof data === 'object' && 'items' in data) {
    const raw = (data as { items?: unknown }).items;
    if (Array.isArray(raw)) {
      return raw as T[];
    }
  }
  return [];
}
