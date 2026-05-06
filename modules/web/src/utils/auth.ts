const TOKEN_KEY = 'go-devops-token';

export function getToken(): string {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || '';
  } catch (err) {
    console.warn('read token failed', err);
    return '';
  }
}

export function setToken(token: string | null | undefined): void {
  try {
    window.localStorage.setItem(TOKEN_KEY, token || '');
  } catch (err) {
    console.warn('persist token failed', err);
  }
}

export function clearToken(): void {
  try {
    window.localStorage.removeItem(TOKEN_KEY);
  } catch (err) {
    console.warn('clear token failed', err);
  }
}

export function syncTokenFromUrl(): string {
  if (typeof window === 'undefined') {
    return '';
  }
  const url = new URL(window.location.href);
  const token = url.searchParams.get('token');
  if (!token) {
    return '';
  }
  setToken(token);
  url.searchParams.delete('token');
  const cleanURL = `${url.pathname}${url.search}${url.hash}`;
  window.history.replaceState({}, document.title, cleanURL);
  return token;
}
