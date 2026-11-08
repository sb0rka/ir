import { useState } from 'react'
import { LoaderCircle } from 'lucide-react'

export function LoginPage({ error, onLogin, loading = false }: { error: string | null; onLogin: () => Promise<void>; loading?: boolean }) {
  const [submitting, setSubmitting] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)
  const pending = loading || submitting
  const alertMessage = !submitting ? localError || error : null

  return (
    <main className="grid min-h-screen place-items-center bg-surface-0 text-fg">
      <div className="relative w-max min-w-[280px]">
        <button
          type="button"
          className="inline-flex h-10 min-w-[280px] items-center justify-center whitespace-nowrap rounded-xs bg-fg/90 px-8 text-sm font-medium text-surface-0 shadow transition-colors hover:bg-fg focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-fg active:scale-[0.97] disabled:pointer-events-none disabled:opacity-50"
          disabled={pending}
          onClick={() => {
            setSubmitting(true)
            setLocalError(null)
            void onLogin().catch((reason: unknown) => {
              setLocalError(reason instanceof Error ? reason.message : 'Не удалось войти')
              setSubmitting(false)
            })
          }}
        >
          <span className="flex items-center justify-center gap-4">
            <span className="flex h-[15px] w-7 shrink-0 items-center justify-center">
              {pending ? (
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <span className="h-[15px] w-7" aria-hidden="true">
                  <img src="https://static.sb0rka.ru/favicon-dark.svg" alt="" className="h-full w-full object-contain dark:hidden" referrerPolicy="no-referrer" />
                  <img src="https://static.sb0rka.ru/favicon-light.svg" alt="" className="hidden h-full w-full object-contain dark:block" referrerPolicy="no-referrer" />
                </span>
              )}
            </span>
            <span className="truncate">{submitting ? 'Переход в Sb0rka...' : 'Вход с помощью Sb0rka'}</span>
          </span>
        </button>
        {(loading || alertMessage) && (
          <div className="absolute inset-x-0 top-full mt-6 space-y-2">
            {loading && (
              <p className="text-sm font-normal text-fg-muted" aria-live="polite">
                Проверка сессии…
              </p>
            )}
            {alertMessage && (
              <p role="alert" className="text-sm font-normal text-critical">
                {alertMessage}
              </p>
            )}
          </div>
        )}
      </div>
    </main>
  )
}
