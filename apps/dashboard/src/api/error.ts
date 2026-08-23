export type ApiErrorCode =
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'validation'
  | 'source_unavailable'
  | 'source_auth_failed'
  | 'not_implemented'
  | 'internal'
  | 'bad_request'
  | 'timeout'
  | 'unknown'

export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly status: number
  readonly details?: Record<string, unknown>

  constructor(
    code: ApiErrorCode,
    message: string,
    status: number,
    details?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.details = details
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}

export function isNotImplemented(err: unknown): boolean {
  return isApiError(err) && err.code === 'not_implemented'
}

export function isUnauthorized(err: unknown): boolean {
  return isApiError(err) && (err.code === 'unauthorized' || err.status === 401)
}

type Envelope = {
  error?: {
    code?: string
    message?: string
    details?: Record<string, unknown>
  }
}

function asCode(value: string | undefined): ApiErrorCode {
  switch (value) {
    case 'unauthorized':
    case 'forbidden':
    case 'not_found':
    case 'conflict':
    case 'validation':
    case 'source_unavailable':
    case 'source_auth_failed':
    case 'not_implemented':
    case 'internal':
    case 'bad_request':
    case 'timeout':
      return value
    default:
      return 'unknown'
  }
}

export function unwrapError(error: unknown, status = 0): ApiError {
  if (error instanceof ApiError) return error
  const envelope = error as Envelope | undefined
  const code = asCode(envelope?.error?.code)
  const message = envelope?.error?.message || 'Неизвестная ошибка API'
  return new ApiError(code, message, status, envelope?.error?.details)
}

export function errorMessage(err: unknown): string {
  if (isNotImplemented(err)) {
    return 'Операция есть в контракте, но сервер еще отвечает 501 — действие не применено'
  }
  if (isUnauthorized(err)) {
    return 'Сессия истекла или токен был отклонен. Войдите снова'
  }
  if (isApiError(err)) return err.message
  if (err instanceof Error) return err.message
  return 'Неизвестная ошибка'
}
