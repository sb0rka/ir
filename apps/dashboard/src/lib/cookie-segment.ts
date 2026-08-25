/** Normalize one Cookie-header pair for a known field name. */
export function normalizeCookieSegment(field: string, raw: string): string | null {
    let value = stripWrappingQuotes(raw.trim())
    if (value === '') {
      return null
    }
    const prefix = `${field}=`
    if (value.startsWith(prefix)) {
      value = value.slice(prefix.length)
    }
    return `${field}=${value}`
  }
  
  /** Reject pastes that are not a single cookie value for `field`. */
  export function cookieSegmentError(field: string, raw: string): string | null {
    let value = stripWrappingQuotes(raw.trim())
    if (value === '') {
      return null
    }
    const prefix = `${field}=`
    if (value.startsWith(prefix)) {
      value = value.slice(prefix.length)
    }
    if (value.includes(';') || /\s/.test(value) || /["']/.test(value)) {
      return `В поле ${field} укажите только значение cookie (без кавычек и других пар)`
    }
    // Env-line paste: Name=... without being this field.
    const foreign = value.match(/^([A-Za-z_][\w.]*)=/)
    if (foreign && foreign[1] !== field) {
      return `В поле ${field} обнаружена чужая пара ${foreign[1]}=...`
    }
    return null
  }
  
  function stripWrappingQuotes(value: string): string {
    if (value.length >= 2) {
      const start = value[0]
      const end = value[value.length - 1]
      if ((start === '"' && end === '"') || (start === "'" && end === "'")) {
        return value.slice(1, -1).trim()
      }
    }
    return value
  }
  