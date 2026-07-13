/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- DefineComponent requires `any` for the generic instance type slot
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, any>
  export default component
}

declare global {
  interface ImportMetaEnv {
    readonly VITE_API_BASE_URL?: string
    readonly VITE_PUBLIC_ASSET_BASE_URL?: string
    readonly VITE_CANVAS_PERF_PROBE?: string
  }
}

export {}
