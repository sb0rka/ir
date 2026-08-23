export function validatePTURL(value: string): string | null {
  if (!value) return null
  try {
    const url = new URL(value)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      return 'Допустимы только http:// и https://'
    }
    if (url.username || url.password || url.search || url.hash) {
      return 'URL не должен содержать credentials, query или fragment'
    }
    const path = url.pathname.replace(/\/+$/, '').toLowerCase()
    if (path.endsWith('/api/account/userinfo')) {
      return 'Укажите базовый URL без /api/account/userinfo'
    }
    return null
  } catch {
    return 'Введите корректный URL'
  }
}
