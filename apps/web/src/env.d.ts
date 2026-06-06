/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, any>
  export default component
}

declare global {
  interface ImportMetaEnv {
    readonly VITE_API_BASE_URL?: string
    readonly VITE_AUTH_BASE_URL?: string
    readonly VITE_ADMIN_MOCK_DATA?: string
    readonly VITE_PUBLIC_ASSET_BASE_URL?: string
    readonly VITE_WORKSPACE_FLOW_ENABLE_CULLING?: string
  }

  interface Window {
    $dialog: any
    $message: any
  }
}

export {}
