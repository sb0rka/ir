export const PT_SIEM_COOKIE_E = 'e'
export const PT_SIEM_COOKIE_IDSRV_SESSION = 'idsrv.session'
export const PT_SIEM_COOKIE_IDSRV = 'idsrv'
export const PT_SIEM_COOKIE_PORTAL = 'IncidentManagementPortalCookie'

export type PtSiemCookieField =
  | typeof PT_SIEM_COOKIE_E
  | typeof PT_SIEM_COOKIE_IDSRV_SESSION
  | typeof PT_SIEM_COOKIE_IDSRV
  | typeof PT_SIEM_COOKIE_PORTAL

export type PtSiemCookieParts = Record<PtSiemCookieField, string>

const eventsFields: PtSiemCookieField[] = [
  PT_SIEM_COOKIE_E,
  PT_SIEM_COOKIE_IDSRV_SESSION,
  PT_SIEM_COOKIE_IDSRV,
]

export function buildPtSiemCookie(parts: PtSiemCookieParts): string {
  const segments: string[] = []
  for (const field of [...eventsFields, PT_SIEM_COOKIE_PORTAL]) {
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

export function validatePtSiemCookieParts(parts: PtSiemCookieParts): string | null {
  const values = Object.values(parts).map((value) => value.trim())
  if (values.every((value) => value === '')) {
    return null
  }
  const eventsFilled = eventsFields.filter((field) => parts[field].trim() !== '').length
  if (eventsFilled > 0 && eventsFilled < eventsFields.length) {
    return 'Для Events API заполните e, idsrv.session и idsrv'
  }
  if (parts[PT_SIEM_COOKIE_PORTAL].trim() === '' && eventsFilled === 0) {
    return 'Укажите cookie для Events API или IncidentManagementPortalCookie'
  }
  return null
}
