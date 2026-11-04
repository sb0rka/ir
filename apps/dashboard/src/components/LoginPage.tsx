import { useState } from 'react'
import { Button } from './ui'

export function LoginPage({ error, onLogin }: { error: string | null; onLogin: () => Promise<void> }) {
  const [submitting, setSubmitting] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)
  return (
    <main className="flex min-h-full items-center justify-center px-4 py-10 text-fg">
      <div className="w-full max-w-sm space-y-4">
        <p>Войдите в IR через Sb0rka.</p>
        {(localError || error) && <p role="alert" className="text-sm text-critical">{localError || error}</p>}
        <Button className="w-full" variant="primary" disabled={submitting} onClick={() => {
          setSubmitting(true)
          void onLogin().catch((reason: unknown) => { setLocalError(reason instanceof Error ? reason.message : 'Не удалось войти'); setSubmitting(false) })
        }}>{submitting ? 'Переход к входу…' : 'Войти через Sb0rka'}</Button>
      </div>
    </main>
  )
}
