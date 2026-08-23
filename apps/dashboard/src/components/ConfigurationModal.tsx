import { useEffect, useMemo, useState } from 'react'
import { Check, CircleAlert, LoaderCircle, Settings2, X } from 'lucide-react'
import {
  listSomBoards,
  listSomWorkspaces,
  probePTUser,
  type SomBoardOption,
  type SomWorkspaceOption,
} from '../api/integrations'
import {
  currentSecretVersionCreatedAt,
  listSecrets,
  secretAgeHours,
  writeSecret,
  type Project,
  type SecretMetadata,
} from '../api/platform'
import { Button } from './ui'
import { clsx } from '../lib/utils'
import {
  PT_COOKIE_FIELD_IDSRV,
  PT_COOKIE_FIELD_IDSRV_SESSION,
  PT_COOKIE_FIELD_PORTAL,
  buildPtCookie,
  validatePtCookieParts,
  type PtCookieField,
} from '../lib/pt-cookie'
import { validatePTURL } from '../lib/validation'
import {
  DEFAULT_SOM_MODEL_ID,
  DEFAULT_SOM_VARIANT,
  getSomSelectors,
  getSomRunSettings,
  setSomBoardSelector,
  setSomRunSettings,
  setSomWorkspaceSelector,
} from '../api/som-settings'

import { withTimeout } from '../lib/with-timeout'

const PT_PROBE_TIMEOUT_MS = 20_000
const SECRET_METADATA_TIMEOUT_MS = 10_000
const SOM_SECRET = 'DEMO_SOM_ACCESS_TOKEN'
const SOM_VARIANT_KEY = 'som_variant'
const SOM_MODEL_ID_KEY = 'som_model_id'
const PT_COOKIE_SECRET = 'DEMO_PT_COOKIE'
const PT_URL_SECRET = 'DEMO_PT_SIEM_BASE_URL'

type SecretName =
  | typeof SOM_SECRET
  | typeof PT_COOKIE_SECRET
  | typeof PT_URL_SECRET
type FieldState = 'idle' | 'saving' | 'saved' | 'error'

interface Drafts {
  [SOM_SECRET]: string
  [SOM_VARIANT_KEY]: string
  [SOM_MODEL_ID_KEY]: string
  [PT_URL_SECRET]: string
  [PT_COOKIE_FIELD_IDSRV_SESSION]: string
  [PT_COOKIE_FIELD_IDSRV]: string
  [PT_COOKIE_FIELD_PORTAL]: string
}

type DraftName = keyof Drafts

const emptyDrafts: Drafts = {
  [SOM_SECRET]: '',
  [SOM_VARIANT_KEY]: '',
  [SOM_MODEL_ID_KEY]: '',
  [PT_URL_SECRET]: '',
  [PT_COOKIE_FIELD_IDSRV_SESSION]: '',
  [PT_COOKIE_FIELD_IDSRV]: '',
  [PT_COOKIE_FIELD_PORTAL]: '',
}

const emptyStates: Record<SecretName, FieldState> = {
  [SOM_SECRET]: 'idle',
  [PT_COOKIE_SECRET]: 'idle',
  [PT_URL_SECRET]: 'idle',
}

function isSecretName(name: DraftName): name is typeof SOM_SECRET | typeof PT_URL_SECRET {
  return name === SOM_SECRET || name === PT_URL_SECRET
}

function isPtCookieField(name: DraftName): name is PtCookieField {
  return (
    name === PT_COOKIE_FIELD_IDSRV_SESSION ||
    name === PT_COOKIE_FIELD_IDSRV ||
    name === PT_COOKIE_FIELD_PORTAL
  )
}

function ageLabel(createdAt: string | null): string {
  const hours = secretAgeHours(createdAt)
  if (hours == null) return 'не задана'
  if (hours === 0) return '<1 ч назад'
  return `${hours} ч назад`
}

function FieldStatus({ state }: { state: FieldState }) {
  if (state === 'saving') return <LoaderCircle className="h-3.5 w-3.5 animate-spin text-fg-dim" />
  if (state === 'saved') return <Check className="h-3.5 w-3.5 text-confirmed" />
  if (state === 'error') return <CircleAlert className="h-3.5 w-3.5 text-critical" />
  return null
}

interface SomOptions {
  workspaces: SomWorkspaceOption[]
  workspaceId: string
  boards: SomBoardOption[]
  boardId: string
}

async function loadSomOptions(projectId: string, resetSelection: boolean): Promise<SomOptions> {
  const workspaces = await listSomWorkspaces(projectId)
  const stored = resetSelection ? { workspace: null, board: null } : getSomSelectors(projectId)
  const workspace =
    (stored.workspace
      ? workspaces.find(
          (item) =>
            item.id === stored.workspace ||
            item.name === stored.workspace ||
            item.slug === stored.workspace,
        )
      : undefined) ?? workspaces[0]
  if (!workspace) {
    setSomWorkspaceSelector(projectId, null)
    setSomBoardSelector(projectId, null)
    return { workspaces, workspaceId: '', boards: [], boardId: '' }
  }

  setSomWorkspaceSelector(projectId, workspace.id)
  const boards = await listSomBoards(projectId, workspace.id)
  const board =
    (stored.board
      ? boards.find((item) => item.id === stored.board || item.name === stored.board)
      : undefined) ?? boards[0]
  setSomBoardSelector(projectId, board?.id ?? null)
  return {
    workspaces,
    workspaceId: workspace.id,
    boards,
    boardId: board?.id ?? '',
  }
}

export function ConfigurationModal({
  projects,
  currentProjectId,
  onClose,
}: {
  projects: Project[]
  currentProjectId: string
  onClose: (appliedProjectId?: string) => void
}) {
  const [projectId, setProjectId] = useState(currentProjectId)
  const [drafts, setDrafts] = useState<Drafts>(emptyDrafts)
  const [states, setStates] = useState<Record<SecretName, FieldState>>(emptyStates)
  const [errors, setErrors] = useState<Partial<Record<SecretName | 'pt' | 'som', string>>>({})
  const [cookieCreatedAt, setCookieCreatedAt] = useState<string | null>(null)
  const [userName, setUserName] = useState<string | null>(null)
  const [ptLoading, setPtLoading] = useState(true)
  const [cookieMetaLoading, setCookieMetaLoading] = useState(true)
  const [somLoading, setSomLoading] = useState(true)
  const [somWorkspaces, setSomWorkspaces] = useState<SomWorkspaceOption[]>([])
  const [somWorkspaceId, setSomWorkspaceId] = useState('')
  const [somBoards, setSomBoards] = useState<SomBoardOption[]>([])
  const [somBoardId, setSomBoardId] = useState('')
  const [saving, setSaving] = useState(false)
  const [summary, setSummary] = useState<string | null>(null)

  const project = useMemo(
    () => projects.find((item) => item.id === projectId),
    [projectId, projects],
  )

  useEffect(() => {
    let cancelled = false
    const somRunSettings = getSomRunSettings(projectId)
    setDrafts({
      ...emptyDrafts,
      [SOM_VARIANT_KEY]: somRunSettings.variant,
      [SOM_MODEL_ID_KEY]: somRunSettings.modelId,
    })
    setStates(emptyStates)
    setErrors({})
    setSummary(null)
    setCookieCreatedAt(null)
    setUserName(null)
    setPtLoading(true)
    setCookieMetaLoading(true)
    setSomLoading(true)
    setSomWorkspaces([])
    setSomWorkspaceId('')
    setSomBoards([])
    setSomBoardId('')

    void withTimeout(currentSecretVersionCreatedAt(projectId, PT_COOKIE_SECRET), SECRET_METADATA_TIMEOUT_MS)
      .then((createdAt) => {
        if (!cancelled) setCookieCreatedAt(createdAt)
      })
      .catch(() => {
        if (!cancelled) setCookieCreatedAt(null)
      })
      .finally(() => {
        if (!cancelled) setCookieMetaLoading(false)
      })

    void withTimeout(probePTUser(projectId), PT_PROBE_TIMEOUT_MS)
      .then((name) => {
        if (!cancelled) {
          setUserName(name)
          setErrors((current) => ({ ...current, pt: undefined }))
        }
      })
      .catch((reason: unknown) => {
        if (!cancelled) {
          setUserName(null)
          setErrors((current) => ({
            ...current,
            pt: reason instanceof Error ? reason.message : 'PT SIEM недоступен',
          }))
        }
      })
      .finally(() => {
        if (!cancelled) setPtLoading(false)
      })
    void loadSomOptions(projectId, false)
      .then((options) => {
        if (cancelled) return
        setSomWorkspaces(options.workspaces)
        setSomWorkspaceId(options.workspaceId)
        setSomBoards(options.boards)
        setSomBoardId(options.boardId)
      })
      .catch((reason: unknown) => {
        if (cancelled) return
        setErrors((current) => ({
          ...current,
          som: reason instanceof Error ? reason.message : 'SOM недоступен',
        }))
      })
      .finally(() => {
        if (!cancelled) setSomLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  const setDraft = (name: DraftName, value: string) => {
    setDrafts((current) => ({ ...current, [name]: value }))
    if (isSecretName(name)) {
      setStates((current) => ({ ...current, [name]: 'idle' }))
      setErrors((current) => ({ ...current, [name]: undefined }))
    }
    if (isPtCookieField(name)) {
      setStates((current) => ({ ...current, [PT_COOKIE_SECRET]: 'idle' }))
      setErrors((current) => ({ ...current, [PT_COOKIE_SECRET]: undefined }))
    }
  }

  const selectSomWorkspace = async (workspaceId: string) => {
    const workspace = somWorkspaces.find((item) => item.id === workspaceId)
    setSomWorkspaceId(workspaceId)
    setSomBoardId('')
    setSomBoards([])
    setSomWorkspaceSelector(projectId, workspace?.id ?? null)
    setSomBoardSelector(projectId, null)
    if (!workspace) return
    setSomLoading(true)
    try {
      const boards = await listSomBoards(projectId, workspace.id)
      const board = boards[0]
      setSomBoards(boards)
      setSomBoardId(board?.id ?? '')
      setSomBoardSelector(projectId, board?.id ?? null)
      setErrors((current) => ({ ...current, som: undefined }))
    } catch (reason) {
      setErrors((current) => ({
        ...current,
        som: reason instanceof Error ? reason.message : 'Не удалось загрузить boards',
      }))
    } finally {
      setSomLoading(false)
    }
  }

  const selectSomBoard = (boardId: string) => {
    const board = somBoards.find((item) => item.id === boardId)
    setSomBoardId(boardId)
    setSomBoardSelector(projectId, board?.id ?? null)
  }

  const save = async () => {
    const cookieParts = {
      [PT_COOKIE_FIELD_IDSRV_SESSION]: drafts[PT_COOKIE_FIELD_IDSRV_SESSION].trim(),
      [PT_COOKIE_FIELD_IDSRV]: drafts[PT_COOKIE_FIELD_IDSRV].trim(),
      [PT_COOKIE_FIELD_PORTAL]: drafts[PT_COOKIE_FIELD_PORTAL].trim(),
    }
    const cookieError = validatePtCookieParts(cookieParts)
    if (cookieError) {
      setStates((current) => ({ ...current, [PT_COOKIE_SECRET]: 'error' }))
      setErrors((current) => ({ ...current, [PT_COOKIE_SECRET]: cookieError }))
      return
    }
    const ptCookie = Object.values(cookieParts).every(Boolean) ? buildPtCookie(cookieParts) : ''

    const values = {
      [SOM_SECRET]: drafts[SOM_SECRET].trim(),
      [SOM_VARIANT_KEY]: drafts[SOM_VARIANT_KEY].trim(),
      [SOM_MODEL_ID_KEY]: drafts[SOM_MODEL_ID_KEY].trim(),
      [PT_URL_SECRET]: drafts[PT_URL_SECRET].trim(),
      [PT_COOKIE_SECRET]: ptCookie,
    }
    const urlError = validatePTURL(values[PT_URL_SECRET])
    if (urlError) {
      setStates((current) => ({ ...current, [PT_URL_SECRET]: 'error' }))
      setErrors((current) => ({ ...current, [PT_URL_SECRET]: urlError }))
      return
    }

    setSaving(true)
    setSummary(null)
    setErrors({})
    const ordered: SecretName[] = [PT_URL_SECRET, PT_COOKIE_SECRET, SOM_SECRET]
    let existing: SecretMetadata[] = []
    if (ordered.some((name) => values[name])) {
      try {
        existing = await listSecrets(projectId)
      } catch (reason) {
        setSummary(reason instanceof Error ? reason.message : 'Не удалось загрузить Secrets')
        setSaving(false)
        return
      }
    }
    const savedNames = new Set<SecretName>()
    const failedNames = new Set<SecretName>()
    let savedCount = 0
    for (const name of ordered) {
      if (!values[name]) continue
      setStates((current) => ({ ...current, [name]: 'saving' }))
      try {
        await writeSecret(
          projectId,
          name,
          values[name],
          existing.find((secret) => secret.name === name),
        )
        savedCount += 1
        savedNames.add(name)
        setStates((current) => ({ ...current, [name]: 'saved' }))
      } catch (reason) {
        failedNames.add(name)
        setStates((current) => ({ ...current, [name]: 'error' }))
        setErrors((current) => ({
          ...current,
          [name]: reason instanceof Error ? reason.message : 'Ошибка сохранения',
        }))
      }
    }

    const projectChanged = projectId !== currentProjectId
    const ptChanged =
      savedNames.has(PT_URL_SECRET) || savedNames.has(PT_COOKIE_SECRET)
    if (ptChanged || projectChanged) {
      setPtLoading(true)
      try {
        setUserName(await withTimeout(probePTUser(projectId), PT_PROBE_TIMEOUT_MS))
        setErrors((current) => ({ ...current, pt: undefined }))
      } catch (reason) {
        setUserName(null)
        setErrors((current) => ({
          ...current,
          pt: reason instanceof Error ? reason.message : 'PT SIEM недоступен',
        }))
      } finally {
        setPtLoading(false)
      }
    }

    const somChanged = savedNames.has(SOM_SECRET)
    if (somChanged || projectChanged) {
      setSomLoading(true)
      try {
        const options = await loadSomOptions(projectId, savedNames.has(SOM_SECRET))
        setSomWorkspaces(options.workspaces)
        setSomWorkspaceId(options.workspaceId)
        setSomBoards(options.boards)
        setSomBoardId(options.boardId)
        setErrors((current) => ({ ...current, som: undefined }))
      } catch (reason) {
        setErrors((current) => ({
          ...current,
          som: reason instanceof Error ? reason.message : 'SOM недоступен',
        }))
      } finally {
        setSomLoading(false)
      }
    }

    try {
      setCookieCreatedAt(
        await withTimeout(
          currentSecretVersionCreatedAt(projectId, PT_COOKIE_SECRET),
          SECRET_METADATA_TIMEOUT_MS,
        ),
      )
    } catch {
      // Saving already has per-field results; unavailable metadata must not hide them.
    } finally {
      setCookieMetaLoading(false)
    }
    const somRunSettings = {
      variant: values[SOM_VARIANT_KEY] || DEFAULT_SOM_VARIANT,
      modelId: values[SOM_MODEL_ID_KEY] || DEFAULT_SOM_MODEL_ID,
    }
    setSomRunSettings(projectId, somRunSettings)
    setDrafts({
      [SOM_SECRET]: failedNames.has(SOM_SECRET) ? values[SOM_SECRET] : '',
      [SOM_VARIANT_KEY]: somRunSettings.variant,
      [SOM_MODEL_ID_KEY]: somRunSettings.modelId,
      [PT_URL_SECRET]: failedNames.has(PT_URL_SECRET) ? values[PT_URL_SECRET] : '',
      [PT_COOKIE_FIELD_IDSRV_SESSION]: failedNames.has(PT_COOKIE_SECRET)
        ? cookieParts[PT_COOKIE_FIELD_IDSRV_SESSION]
        : '',
      [PT_COOKIE_FIELD_IDSRV]: failedNames.has(PT_COOKIE_SECRET)
        ? cookieParts[PT_COOKIE_FIELD_IDSRV]
        : '',
      [PT_COOKIE_FIELD_PORTAL]: failedNames.has(PT_COOKIE_SECRET)
        ? cookieParts[PT_COOKIE_FIELD_PORTAL]
        : '',
    })
    setSummary(
      savedCount ? `Сохранено новых версий: ${savedCount}` : 'Конфигурация сохранена',
    )
    setSaving(false)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label="Конфигурация интеграций"
    >
      <div className="flex max-h-[92vh] w-full max-w-2xl flex-col overflow-hidden rounded border border-border-strong bg-surface-1 shadow-2xl shadow-black">
        <div className="flex items-center justify-between border-b border-border bg-surface-2 px-5 py-3">
          <div className="flex items-center gap-2.5">
            <Settings2 className="h-4 w-4 text-fg-muted" />
            <h2 className="text-sm font-semibold">Конфигурация</h2>
          </div>
          <button
            type="button"
            className="rounded p-1 text-fg-dim hover:bg-surface-3 hover:text-fg disabled:opacity-40"
            disabled={saving}
            onClick={() => onClose(projectId)}
            aria-label="Закрыть"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="overflow-y-auto p-5">
          <label className="block space-y-1.5">
            <span className="text-[10px] font-medium text-fg-dim">Проект</span>
            <select
              className="w-full rounded border border-border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none focus:border-fg/40"
              value={projectId}
              disabled={saving}
              onChange={(event) => setProjectId(event.target.value)}
            >
              {projects.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            <p className="text-[11px] text-fg-dim">
              Project ID: <span className="font-mono text-fg-muted">{project?.id}</span>
            </p>
          </label>

          <div className="my-5 border-t border-border" />

          <div className="space-y-4">
            <SecretField
              label={PT_URL_SECRET}
              value={drafts[PT_URL_SECRET]}
              state={states[PT_URL_SECRET]}
              error={errors[PT_URL_SECRET]}
              technicalLabel
              placeholder="https://siem.example.local"
              onChange={(value) => setDraft(PT_URL_SECRET, value)}
            />
            <div className="space-y-2">
              <CookiePartInput
                placeholder={PT_COOKIE_FIELD_IDSRV_SESSION}
                value={drafts[PT_COOKIE_FIELD_IDSRV_SESSION]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_COOKIE_FIELD_IDSRV_SESSION, value)}
              />
              <CookiePartInput
                placeholder={PT_COOKIE_FIELD_IDSRV}
                value={drafts[PT_COOKIE_FIELD_IDSRV]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_COOKIE_FIELD_IDSRV, value)}
              />
              <CookiePartInput
                placeholder={PT_COOKIE_FIELD_PORTAL}
                value={drafts[PT_COOKIE_FIELD_PORTAL]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_COOKIE_FIELD_PORTAL, value)}
              />
              {(errors[PT_COOKIE_SECRET] || states[PT_COOKIE_SECRET] !== 'idle') && (
                <div className="flex items-center gap-2">
                  <FieldStatus state={states[PT_COOKIE_SECRET]} />
                  {errors[PT_COOKIE_SECRET] && (
                    <p className="text-[11px] text-critical">{errors[PT_COOKIE_SECRET]}</p>
                  )}
                </div>
              )}
            </div>

            <div className="grid gap-2 rounded border border-border bg-surface-0 p-3 sm:grid-cols-2">
              <div>
                <div className="text-[10px] font-medium text-fg-dim">PT user</div>
                <div className="mt-1 font-mono text-sm">
                  {ptLoading ? (
                    <LoaderCircle className="h-3.5 w-3.5 animate-spin text-fg-dim" />
                  ) : (
                    userName ?? 'не определен'
                  )}
                </div>
                {errors.pt && <p className="mt-1 text-[11px] text-critical">{errors.pt}</p>}
              </div>
              <div>
                <div className="text-[10px] font-medium text-fg-dim">
                  Обновление cookie
                </div>
                <div
                  className={clsx(
                    'mt-1 font-mono text-sm',
                    userName ? 'text-fg-muted' : 'text-critical',
                  )}
                >
                  {cookieMetaLoading ? 'проверка…' : ageLabel(cookieCreatedAt)}
                </div>
              </div>
            </div>

            <div className="border-t border-border pt-4">
              <SecretField
                label={SOM_SECRET}
                value={drafts[SOM_SECRET]}
                state={states[SOM_SECRET]}
                error={errors[SOM_SECRET]}
                secret
                technicalLabel
                placeholder="Новый access token"
                onChange={(value) => setDraft(SOM_SECRET, value)}
              />
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <label className="block space-y-1.5">
                  <span className="text-[10px] font-medium text-fg-dim">SOM workspace</span>
                  <select
                    className="w-full rounded border border-border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none focus:border-fg/40 disabled:opacity-50"
                    value={somWorkspaceId}
                    disabled={somLoading || somWorkspaces.length === 0}
                    onChange={(event) => void selectSomWorkspace(event.target.value)}
                  >
                    {somWorkspaces.length === 0 && <option value="">SOM token недоступен</option>}
                    {somWorkspaces.map((workspace) => (
                      <option key={workspace.id} value={workspace.id}>
                        {workspace.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="block space-y-1.5">
                  <span className="text-[10px] font-medium text-fg-dim">SOM board</span>
                  <select
                    className="w-full rounded border border-border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none focus:border-fg/40 disabled:opacity-50"
                    value={somBoardId}
                    disabled={somLoading || somBoards.length === 0}
                    onChange={(event) => selectSomBoard(event.target.value)}
                  >
                    {somBoards.length === 0 && <option value="">Board недоступен</option>}
                    {somBoards.map((board) => (
                      <option key={board.id} value={board.id}>
                        {board.name}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <SecretField
                  label="SOM model variant"
                  value={drafts[SOM_VARIANT_KEY]}
                  state="idle"
                  inputType="text"
                  placeholder={DEFAULT_SOM_VARIANT}
                  onChange={(value) => setDraft(SOM_VARIANT_KEY, value)}
                />
                <SecretField
                  label="SOM model ID"
                  value={drafts[SOM_MODEL_ID_KEY]}
                  state="idle"
                  inputType="text"
                  placeholder={DEFAULT_SOM_MODEL_ID}
                  onChange={(value) => setDraft(SOM_MODEL_ID_KEY, value)}
                />
              </div>
              {errors.som && <p className="mt-2 text-[11px] text-critical">{errors.som}</p>}
            </div>
          </div>

          {summary && (
            <div className="mt-3 rounded border border-confirmed/30 bg-confirmed/10 px-3 py-2 text-xs text-confirmed">
              {summary}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border bg-surface-2 px-5 py-3">
          <Button disabled={saving} variant="ghost" onClick={() => onClose(projectId)}>
            Закрыть
          </Button>
          <Button disabled={saving} variant="primary" onClick={() => void save()}>
            {saving && <LoaderCircle className="h-3.5 w-3.5 animate-spin" />}
            Сохранить и проверить
          </Button>
        </div>
      </div>
    </div>
  )
}

function CookiePartInput({
  placeholder,
  value,
  state,
  onChange,
}: {
  placeholder: string
  value: string
  state: FieldState
  onChange: (value: string) => void
}) {
  return (
    <input
      autoComplete="off"
      className={clsx(
        'w-full rounded border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none placeholder:text-fg-dim focus:border-fg/40',
        state === 'error' ? 'border-critical/70' : 'border-border',
      )}
      type="password"
      value={value}
      placeholder={placeholder}
      onChange={(event) => onChange(event.target.value)}
    />
  )
}

function SecretField({
  label,
  value,
  state,
  error,
  placeholder,
  secret = false,
  inputType,
  technicalLabel = false,
  onChange,
}: {
  label: string
  value: string
  state: FieldState
  error?: string
  placeholder: string
  secret?: boolean
  inputType?: 'text' | 'url'
  technicalLabel?: boolean
  onChange: (value: string) => void
}) {
  return (
    <label className="block space-y-1.5">
      <span
        className={clsx(
          'flex items-center gap-2 text-[10px] text-fg-dim',
          technicalLabel ? 'font-semibold uppercase tracking-wider' : 'font-medium',
        )}
      >
        {label}
        <FieldStatus state={state} />
      </span>
      <input
        autoComplete="off"
        className={clsx(
          'w-full rounded border bg-surface-0 px-3 py-2 font-mono text-sm text-fg outline-none placeholder:text-fg-dim focus:border-fg/40',
          state === 'error' ? 'border-critical/70' : 'border-border',
        )}
        type={secret ? 'password' : (inputType ?? 'url')}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {error && <span className="block text-[11px] text-critical">{error}</span>}
    </label>
  )
}
