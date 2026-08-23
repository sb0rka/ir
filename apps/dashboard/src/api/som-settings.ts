export interface SomSelectors {
  workspace: string | null
  board: string | null
}

export interface SomRunSettings {
  variant: string
  modelId: string
}

const WORKSPACE_SELECTOR = 'som_workspace'
const BOARD_SELECTOR = 'som_board'
const SOM_VARIANT = 'som_variant'
const SOM_MODEL_ID = 'som_model_id'

export const DEFAULT_SOM_VARIANT = 'DEFAULT'
export const DEFAULT_SOM_MODEL_ID = 'mws/som-pt-gpt-oss-120b-mh'

function key(projectId: string, setting: string): string {
  return `ir.${projectId}.${setting}`
}

function readSession(projectId: string, setting: string): string | null {
  try {
    return sessionStorage.getItem(key(projectId, setting))
  } catch {
    return null
  }
}

function writeSession(projectId: string, setting: string, value: string | null): void {
  try {
    if (value) sessionStorage.setItem(key(projectId, setting), value)
    else sessionStorage.removeItem(key(projectId, setting))
  } catch {
    /* The dropdown state remains available in memory for this modal. */
  }
}

function readLocal(projectId: string, setting: string): string | null {
  try {
    return localStorage.getItem(key(projectId, setting))?.trim() || null
  } catch {
    return null
  }
}

function writeLocal(projectId: string, setting: string, value: string): void {
  try {
    localStorage.setItem(key(projectId, setting), value)
  } catch {
    /* The live form still keeps the value when browser storage is unavailable. */
  }
}

export function getSomSelectors(projectId: string): SomSelectors {
  return {
    workspace: readSession(projectId, WORKSPACE_SELECTOR),
    board: readSession(projectId, BOARD_SELECTOR),
  }
}

export function setSomWorkspaceSelector(projectId: string, selector: string | null): void {
  writeSession(projectId, WORKSPACE_SELECTOR, selector)
}

export function setSomBoardSelector(projectId: string, selector: string | null): void {
  writeSession(projectId, BOARD_SELECTOR, selector)
}

export function getSomRunSettings(projectId: string): SomRunSettings {
  return {
    variant: readLocal(projectId, SOM_VARIANT) ?? DEFAULT_SOM_VARIANT,
    modelId: readLocal(projectId, SOM_MODEL_ID) ?? DEFAULT_SOM_MODEL_ID,
  }
}

export function setSomRunSettings(projectId: string, settings: SomRunSettings): void {
  writeLocal(projectId, SOM_VARIANT, settings.variant.trim() || DEFAULT_SOM_VARIANT)
  writeLocal(projectId, SOM_MODEL_ID, settings.modelId.trim() || DEFAULT_SOM_MODEL_ID)
}
