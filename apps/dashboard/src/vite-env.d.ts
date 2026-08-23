/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GATEWAY_URL: string
  readonly VITE_IR_URL: string
  readonly VITE_AUTH_BASE_URL: string
  readonly VITE_PLATFORM_API_BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
