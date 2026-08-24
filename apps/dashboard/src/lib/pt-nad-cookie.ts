export const PT_NAD_COOKIE_SESSIONID = 'sessionid'
export const PT_NAD_COOKIE_CSRFTOKEN = 'csrftoken'

export type PtNadCookieField =
  | typeof PT_NAD_COOKIE_SESSIONID
  | typeof PT_NAD_COOKIE_CSRFTOKEN

export type PtNadCookieParts = Record<PtNadCookieField, string>

const nadFields: PtNadCookieField[] = [PT_NAD_COOKIE_SESSIONID, PT_NAD_COOKIE_CSRFTOKEN]

export function buildPtNadCookie(parts: PtNadCookieParts): string {
  const segments: string[] = []
  for (const field of nadFields) {
    const value = parts[field].trim()
    if (value === '') {
      continue
    }
    if (value.includes('=')) {
      segments.push(value)
      continue
    }
    segments.push(`${field}=${value}`)
  }
  return segments.join('; ')
}

export function validatePtNadCookieParts(parts: PtNadCookieParts): string | null {
  const values = nadFields.map((field) => parts[field].trim())
  if (values.every((value) => value === '')) {
    return null
  }
  if (values.some((value) => value === '')) {
    return 'Для NAD заполните sessionid и csrftoken'
  }
  return null
}
