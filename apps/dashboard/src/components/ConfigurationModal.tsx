import { useEffect, useMemo, useState } from 'react'
import { Check, CircleAlert, LoaderCircle, Settings2, X } from 'lucide-react'
import {
  listProjectSources,
  listSomBoards,
  listSomWorkspaces,
  probeSourceUserinfo,
  type ProjectSource,
  type SomBoardOption,
  type SomWorkspaceOption,
} from '../api/integrations'
import {
  listSecrets,
  writeSecret,
  type Project,
  type SecretMetadata,
} from '../api/platform'
import { Button } from './ui'
import { clsx } from '../lib/utils'
import {
  PT_SIEM_COOKIE_CORE_PORTAL,
  PT_SIEM_COOKIE_IDSRV,
  PT_SIEM_COOKIE_IDSRV_SESSION,
  PT_SIEM_COOKIE_PORTAL,
  buildPtSiemCookie,
  validatePtSiemCookieParts,
  type PtSiemCookieField,
} from '../lib/pt-cookie'
import {
  PT_NAD_COOKIE_CSRFTOKEN,
  PT_NAD_COOKIE_SESSIONID,
  buildPtNadCookie,
  validatePtNadCookieParts,
  type PtNadCookieField,
} from '../lib/pt-nad-cookie'
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
const SOM_SECRET = 'DEMO_SOM_ACCESS_TOKEN'
const SOM_VARIANT_KEY = 'som_variant'
const SOM_MODEL_ID_KEY = 'som_model_id'
const PT_COOKIE_SECRET = 'DEMO_PT_SIEM_COOKIE'
const PT_NAD_COOKIE_SECRET = 'DEMO_PT_NAD_COOKIE'

type SecretName = typeof SOM_SECRET | typeof PT_COOKIE_SECRET | typeof PT_NAD_COOKIE_SECRET
type FieldState = 'idle' | 'saving' | 'saved' | 'error'

interface Drafts {
  [SOM_SECRET]: string
  [SOM_VARIANT_KEY]: string
  [SOM_MODEL_ID_KEY]: string
  [PT_SIEM_COOKIE_CORE_PORTAL]: string
  [PT_SIEM_COOKIE_IDSRV_SESSION]: string
  [PT_SIEM_COOKIE_IDSRV]: string
  [PT_SIEM_COOKIE_PORTAL]: string
  [PT_NAD_COOKIE_SESSIONID]: string
  [PT_NAD_COOKIE_CSRFTOKEN]: string
}

type DraftName = keyof Drafts

const emptyDrafts: Drafts = {
  [SOM_SECRET]: '',
  [SOM_VARIANT_KEY]: '',
  [SOM_MODEL_ID_KEY]: '',
  [PT_SIEM_COOKIE_CORE_PORTAL]: '',
  [PT_SIEM_COOKIE_IDSRV_SESSION]: '',
  [PT_SIEM_COOKIE_IDSRV]: '',
  [PT_SIEM_COOKIE_PORTAL]: '',
  [PT_NAD_COOKIE_SESSIONID]: '',
  [PT_NAD_COOKIE_CSRFTOKEN]: '',
}

const emptyStates: Record<SecretName, FieldState> = {
  [SOM_SECRET]: 'idle',
  [PT_COOKIE_SECRET]: 'idle',
  [PT_NAD_COOKIE_SECRET]: 'idle',
}

interface SourceProbeResult {
  userName?: string
  error?: string
}

type SourceProbes = Record<string, SourceProbeResult>

function isSecretName(name: DraftName): name is typeof SOM_SECRET {
  return name === SOM_SECRET
}

function isPtCookieField(name: DraftName): name is PtSiemCookieField {
  return (
    name === PT_SIEM_COOKIE_CORE_PORTAL ||
    name === PT_SIEM_COOKIE_IDSRV_SESSION ||
    name === PT_SIEM_COOKIE_IDSRV ||
    name === PT_SIEM_COOKIE_PORTAL
  )
}

function isPtNadCookieField(name: DraftName): name is PtNadCookieField {
  return name === PT_NAD_COOKIE_SESSIONID || name === PT_NAD_COOKIE_CSRFTOKEN
}

async function loadSourceStatuses(projectId: string, refresh = false): Promise<{
  sources: ProjectSource[]
  probes: SourceProbes
}> {
  const sources = await listProjectSources(projectId, refresh)
  const probes: SourceProbes = {}
  await Promise.all(
    sources
      .filter((source) => source.capabilities?.includes('account_userinfo'))
      .map(async (source) => {
        try {
          probes[source.code] = {
            userName: await withTimeout(
              probeSourceUserinfo(projectId, source.code),
              PT_PROBE_TIMEOUT_MS,
            ),
          }
        } catch (reason) {
          probes[source.code] = {
            error: reason instanceof Error ? reason.message : 'Не удалось проверить account_userinfo',
          }
        }
      }),
  )
  return { sources, probes }
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
  const [errors, setErrors] = useState<Partial<Record<SecretName | 'som' | 'sources', string>>>({})
  const [sources, setSources] = useState<ProjectSource[]>([])
  const [sourceProbes, setSourceProbes] = useState<SourceProbes>({})
  const [sourcesLoading, setSourcesLoading] = useState(true)
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
    setSources([])
    setSourceProbes({})
    setSourcesLoading(true)
    setSomLoading(true)
    setSomWorkspaces([])
    setSomWorkspaceId('')
    setSomBoards([])
    setSomBoardId('')

    void loadSourceStatuses(projectId)
      .then(({ sources: loadedSources, probes }) => {
        if (!cancelled) {
          setSources(loadedSources)
          setSourceProbes(probes)
          setErrors((current) => ({ ...current, sources: undefined }))
        }
      })
      .catch((reason: unknown) => {
        if (!cancelled) {
          setSources([])
          setSourceProbes({})
          setErrors((current) => ({
            ...current,
            sources: reason instanceof Error ? reason.message : 'Не удалось проверить источники',
          }))
        }
      })
      .finally(() => {
        if (!cancelled) setSourcesLoading(false)
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
    if (isPtNadCookieField(name)) {
      setStates((current) => ({ ...current, [PT_NAD_COOKIE_SECRET]: 'idle' }))
      setErrors((current) => ({ ...current, [PT_NAD_COOKIE_SECRET]: undefined }))
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
      [PT_SIEM_COOKIE_CORE_PORTAL]: drafts[PT_SIEM_COOKIE_CORE_PORTAL].trim(),
      [PT_SIEM_COOKIE_IDSRV_SESSION]: drafts[PT_SIEM_COOKIE_IDSRV_SESSION].trim(),
      [PT_SIEM_COOKIE_IDSRV]: drafts[PT_SIEM_COOKIE_IDSRV].trim(),
      [PT_SIEM_COOKIE_PORTAL]: drafts[PT_SIEM_COOKIE_PORTAL].trim(),
    }
    const cookieError = validatePtSiemCookieParts(cookieParts)
    if (cookieError) {
      setStates((current) => ({ ...current, [PT_COOKIE_SECRET]: 'error' }))
      setErrors((current) => ({ ...current, [PT_COOKIE_SECRET]: cookieError }))
      return
    }
    const ptCookie = buildPtSiemCookie(cookieParts)

    const nadCookieParts = {
      [PT_NAD_COOKIE_SESSIONID]: drafts[PT_NAD_COOKIE_SESSIONID].trim(),
      [PT_NAD_COOKIE_CSRFTOKEN]: drafts[PT_NAD_COOKIE_CSRFTOKEN].trim(),
    }
    const nadCookieError = validatePtNadCookieParts(nadCookieParts)
    if (nadCookieError) {
      setStates((current) => ({ ...current, [PT_NAD_COOKIE_SECRET]: 'error' }))
      setErrors((current) => ({ ...current, [PT_NAD_COOKIE_SECRET]: nadCookieError }))
      return
    }
    const ptNadCookie = buildPtNadCookie(nadCookieParts)

    const values = {
      [SOM_SECRET]: drafts[SOM_SECRET].trim(),
      [SOM_VARIANT_KEY]: drafts[SOM_VARIANT_KEY].trim(),
      [SOM_MODEL_ID_KEY]: drafts[SOM_MODEL_ID_KEY].trim(),
      [PT_COOKIE_SECRET]: ptCookie,
      [PT_NAD_COOKIE_SECRET]: ptNadCookie,
    }

    setSaving(true)
    setSummary(null)
    setErrors({})
    const ordered: SecretName[] = [PT_COOKIE_SECRET, PT_NAD_COOKIE_SECRET, SOM_SECRET]
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
    setSourcesLoading(true)
    try {
      const { sources: loadedSources, probes } = await loadSourceStatuses(projectId, true)
      setSources(loadedSources)
      setSourceProbes(probes)
      setErrors((current) => ({ ...current, sources: undefined }))
    } catch (reason) {
      setSources([])
      setSourceProbes({})
      setErrors((current) => ({
        ...current,
        sources: reason instanceof Error ? reason.message : 'Не удалось проверить источники',
      }))
    } finally {
      setSourcesLoading(false)
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

    const somRunSettings = {
      variant: values[SOM_VARIANT_KEY] || DEFAULT_SOM_VARIANT,
      modelId: values[SOM_MODEL_ID_KEY] || DEFAULT_SOM_MODEL_ID,
    }
    setSomRunSettings(projectId, somRunSettings)
    setDrafts({
      [SOM_SECRET]: failedNames.has(SOM_SECRET) ? values[SOM_SECRET] : '',
      [SOM_VARIANT_KEY]: somRunSettings.variant,
      [SOM_MODEL_ID_KEY]: somRunSettings.modelId,
      [PT_SIEM_COOKIE_CORE_PORTAL]: failedNames.has(PT_COOKIE_SECRET)
        ? cookieParts[PT_SIEM_COOKIE_CORE_PORTAL]
        : '',
      [PT_SIEM_COOKIE_IDSRV_SESSION]: failedNames.has(PT_COOKIE_SECRET)
        ? cookieParts[PT_SIEM_COOKIE_IDSRV_SESSION]
        : '',
      [PT_SIEM_COOKIE_IDSRV]: failedNames.has(PT_COOKIE_SECRET) ? cookieParts[PT_SIEM_COOKIE_IDSRV] : '',
      [PT_SIEM_COOKIE_PORTAL]: failedNames.has(PT_COOKIE_SECRET) ? cookieParts[PT_SIEM_COOKIE_PORTAL] : '',
      [PT_NAD_COOKIE_SESSIONID]: failedNames.has(PT_NAD_COOKIE_SECRET)
        ? nadCookieParts[PT_NAD_COOKIE_SESSIONID]
        : '',
      [PT_NAD_COOKIE_CSRFTOKEN]: failedNames.has(PT_NAD_COOKIE_SECRET)
        ? nadCookieParts[PT_NAD_COOKIE_CSRFTOKEN]
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
            <SourceStatusPanel
              loading={sourcesLoading}
              sources={sources}
              probes={sourceProbes}
              error={errors.sources}
            />

            <div className="space-y-2">
              <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wider text-fg-dim">
                {PT_COOKIE_SECRET}
                <FieldStatus state={states[PT_COOKIE_SECRET]} />
              </div>
              <CookiePartInput
                label={PT_SIEM_COOKIE_CORE_PORTAL}
                placeholder={PT_SIEM_COOKIE_CORE_PORTAL}
                value={drafts[PT_SIEM_COOKIE_CORE_PORTAL]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_SIEM_COOKIE_CORE_PORTAL, value)}
              />
              <CookiePartInput
                label={PT_SIEM_COOKIE_IDSRV_SESSION}
                placeholder={PT_SIEM_COOKIE_IDSRV_SESSION}
                value={drafts[PT_SIEM_COOKIE_IDSRV_SESSION]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_SIEM_COOKIE_IDSRV_SESSION, value)}
              />
              <CookiePartInput
                label={PT_SIEM_COOKIE_IDSRV}
                placeholder={PT_SIEM_COOKIE_IDSRV}
                value={drafts[PT_SIEM_COOKIE_IDSRV]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_SIEM_COOKIE_IDSRV, value)}
              />
              <CookiePartInput
                label={PT_SIEM_COOKIE_PORTAL}
                placeholder={PT_SIEM_COOKIE_PORTAL}
                value={drafts[PT_SIEM_COOKIE_PORTAL]}
                state={states[PT_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_SIEM_COOKIE_PORTAL, value)}
              />
              {errors[PT_COOKIE_SECRET] && (
                <p className="text-[11px] text-critical">{errors[PT_COOKIE_SECRET]}</p>
              )}
            </div>

            <div className="space-y-2">
              <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wider text-fg-dim">
                {PT_NAD_COOKIE_SECRET}
                <FieldStatus state={states[PT_NAD_COOKIE_SECRET]} />
              </div>
              <CookiePartInput
                label={PT_NAD_COOKIE_SESSIONID}
                placeholder={PT_NAD_COOKIE_SESSIONID}
                value={drafts[PT_NAD_COOKIE_SESSIONID]}
                state={states[PT_NAD_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_NAD_COOKIE_SESSIONID, value)}
              />
              <CookiePartInput
                label={PT_NAD_COOKIE_CSRFTOKEN}
                placeholder={PT_NAD_COOKIE_CSRFTOKEN}
                value={drafts[PT_NAD_COOKIE_CSRFTOKEN]}
                state={states[PT_NAD_COOKIE_SECRET]}
                onChange={(value) => setDraft(PT_NAD_COOKIE_CSRFTOKEN, value)}
              />
              {errors[PT_NAD_COOKIE_SECRET] && (
                <p className="text-[11px] text-critical">{errors[PT_NAD_COOKIE_SECRET]}</p>
              )}
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

function SourceStatusPanel({
  loading,
  sources,
  probes,
  error,
}: {
  loading: boolean
  sources: ProjectSource[]
  probes: SourceProbes
  error?: string
}) {
  return (
    <div className="space-y-2 rounded border border-border bg-surface-0 p-3">
      <div className="text-[10px] font-medium text-fg-dim">Источники проекта</div>
      {loading ? (
        <LoaderCircle className="h-3.5 w-3.5 animate-spin text-fg-dim" />
      ) : sources.length === 0 ? (
        <p className="text-sm text-fg-muted">Нет настроенных источников</p>
      ) : (
        <div className="space-y-2">
          {sources.map((source) => (
            <SourceStatusLine
              key={source.code}
              source={source}
              probe={probes[source.code]}
            />
          ))}
        </div>
      )}
      {error && <p className="text-[11px] text-critical">{error}</p>}
    </div>
  )
}

function SourceStatusLine({
  source,
  probe,
}: {
  source: ProjectSource
  probe?: SourceProbeResult
}) {
  const probeFailed = source.capabilities?.includes('account_userinfo') && probe?.error
  const label =
    source.status === 'offline'
      ? 'не в сети'
      : source.status === 'degraded'
        ? 'частично доступен'
        : probeFailed
          ? 'ошибка проверки'
          : 'онлайн'
  const statusClass =
    source.status === 'offline'
      ? 'text-critical'
      : source.status === 'degraded' || probeFailed
        ? 'text-medium'
        : 'text-confirmed'
  return (
    <div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm text-fg">{source.name}</span>
        <span
          className={clsx(
            'shrink-0 text-xs font-medium',
            statusClass,
          )}
        >
          {label}
        </span>
      </div>
      {probe?.userName && <p className="mt-0.5 text-[11px] text-fg-dim">{probe.userName}</p>}
      {probe?.error && <p className="mt-0.5 text-[11px] text-critical">{probe.error}</p>}
    </div>
  )
}

function CookiePartInput({
  label,
  placeholder,
  value,
  state,
  onChange,
}: {
  label?: string
  placeholder: string
  value: string
  state: FieldState
  onChange: (value: string) => void
}) {
  return (
    <label className="block space-y-1">
      {label && <span className="text-[10px] font-medium text-fg-dim">{label}</span>}
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
    </label>
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
