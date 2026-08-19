/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GATEWAY_URL: string
  readonly VITE_IR_URL: string
  readonly VITE_PROJECT_ID: string
  readonly VITE_TOKEN: string
  readonly VITE_SOM_WORKSPACE_SELECTOR: string
  readonly VITE_SOM_BOARD_SELECTOR: string
  readonly VITE_SOM_TOKEN: string
  readonly VITE_SOM_VARIANT: string
  readonly VITE_SOM_MODEL_ID: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
