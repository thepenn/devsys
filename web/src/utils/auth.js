const TOKEN_KEY = 'go-devops-token'

export function getToken() {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || ''
  } catch (err) {
    console.warn('read token failed', err)
    return ''
  }
}

export function setToken(token) {
  try {
    window.localStorage.setItem(TOKEN_KEY, token)
  } catch (err) {
    console.warn('persist token failed', err)
  }
}

export function clearToken() {
  try {
    window.localStorage.removeItem(TOKEN_KEY)
  } catch (err) {
    console.warn('clear token failed', err)
  }
}

export function syncTokenFromUrl() {
  const url = new URL(window.location.href)
  const token = url.searchParams.get('token')
  if (!token) {
    return ''
  }

  setToken(token)
  url.searchParams.delete('token')
  const cleanURL = `${url.pathname}${url.search}${url.hash}`
  window.history.replaceState({}, document.title, cleanURL)
  return token
}
