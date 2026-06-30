import {
  hasInjectionContext,
  inject,
  provide,
  readonly,
  ref,
  type InjectionKey,
  type Ref,
} from 'vue'

export const DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL = 100

export type CanvasViewportContext = {
  zoomLevel: Readonly<Ref<number>>
}

const CANVAS_VIEWPORT_CONTEXT_KEY: InjectionKey<CanvasViewportContext> =
  Symbol('canvas:viewport')

const defaultCanvasViewportContext: CanvasViewportContext = {
  zoomLevel: readonly(ref(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)),
}

export function provideCanvasViewportContext(context: { zoomLevel: Ref<number> }) {
  provide(CANVAS_VIEWPORT_CONTEXT_KEY, {
    zoomLevel: readonly(context.zoomLevel),
  })
}

export function useCanvasViewportContext(): CanvasViewportContext {
  if (!hasInjectionContext()) return defaultCanvasViewportContext
  return inject(CANVAS_VIEWPORT_CONTEXT_KEY, defaultCanvasViewportContext)
}
