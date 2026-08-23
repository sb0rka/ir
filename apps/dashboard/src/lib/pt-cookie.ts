export const PT_COOKIE_FIELD_IDSRV_SESSION = 'idsrv.session'
export const PT_COOKIE_FIELD_IDSRV = 'idsrv'
export const PT_COOKIE_FIELD_PORTAL = 'IncidentManagementPortalCookie'

export type PtCookieField =
  | typeof PT_COOKIE_FIELD_IDSRV_SESSION
  | typeof PT_COOKIE_FIELD_IDSRV
  | typeof PT_COOKIE_FIELD_PORTAL

export type PtCookieParts = Record<PtCookieField, string>

export function buildPtCookie(parts: PtCookieParts): string {
  return [
    `${PT_COOKIE_FIELD_IDSRV_SESSION}=${parts[PT_COOKIE_FIELD_IDSRV_SESSION]}`,
    `${PT_COOKIE_FIELD_IDSRV}=${parts[PT_COOKIE_FIELD_IDSRV]}`,
    `${PT_COOKIE_FIELD_PORTAL}=${parts[PT_COOKIE_FIELD_PORTAL]}`,
  ].join(';')
}

export function validatePtCookieParts(parts: PtCookieParts): string | null {
  const values = Object.values(parts).map((value) => value.trim())
  const filled = values.filter(Boolean).length
  if (filled === 0) return null
  if (filled === 3) return null
  return 'Заполните все три поля cookie'
}
