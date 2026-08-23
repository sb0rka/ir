import { useState } from 'react'
import { Button } from './ui'

export function LoginPage({
  error,
  onLogin,
}: {
  error: string | null
  onLogin: (login: string, password: string) => Promise<void>
}) {
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)

  return (
    <main className="flex min-h-full items-center justify-center px-4 py-10 text-fg">
      <form
        className="w-full max-w-sm space-y-3"
        onSubmit={(event) => {
          event.preventDefault()
          if (!login.trim() || !password) return
          setSubmitting(true)
          setLocalError(null)
          void onLogin(login.trim(), password)
            .catch((reason: unknown) => {
              setLocalError(reason instanceof Error ? reason.message : 'Не удалось войти')
            })
            .finally(() => setSubmitting(false))
        }}
      >
        <input
          autoFocus
          autoComplete="username"
          aria-label="Login или email"
          className="w-full rounded border border-border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none placeholder:text-fg-dim focus:border-fg/40"
          value={login}
          onChange={(event) => setLogin(event.target.value)}
          placeholder="Login или email"
        />
        <input
          autoComplete="current-password"
          aria-label="Пароль"
          className="w-full rounded border border-border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none placeholder:text-fg-dim focus:border-fg/40"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          placeholder="Пароль"
        />

        {(localError || error) && (
          <div className="rounded border border-critical/40 bg-critical/10 px-3 py-2 text-xs text-critical">
            {localError || error}
          </div>
        )}

        <Button
          className="w-full"
          type="submit"
          variant="primary"
          disabled={submitting || !login.trim() || !password}
        >
          {submitting ? 'Вход…' : 'Войти'}
        </Button>
      </form>
    </main>
  )
}
