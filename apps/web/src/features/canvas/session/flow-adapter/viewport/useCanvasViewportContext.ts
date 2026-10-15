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

export function resolveCanvasViewportZoomScale(zoomLevel: number): number {
  const normalizedZoom = Number(zoomLevel) / DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL
  return Number.isFinite(normalizedZoom) && normalizedZoom > 0 ? normalizedZoom : 1
}

export function resolveCanvasNodeUiScale(zoomLevel: number): number {
  return 1 / resolveCanvasViewportZoomScale(zoomLevel)
}

export type CanvasViewportContext = {
  zoomLevel: Readonly<Ref<number>>
  qualityZoomLevel: Readonly<Ref<number>>
}

const CANVAS_VIEWPORT_CONTEXT_KEY: InjectionKey<CanvasViewportContext> = Symbol('canvas:viewport')

const defaultCanvasViewportContext: CanvasViewportContext = {
  zoomLevel: readonly(ref(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)),
  qualityZoomLevel: readonly(ref(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)),
}

export function provideCanvasViewportContext(context: CanvasViewportContext) {
  provide(CANVAS_VIEWPORT_CONTEXT_KEY, {
    zoomLevel: readonly(context.zoomLevel),
    qualityZoomLevel: readonly(context.qualityZoomLevel),
  })
}

export function useCanvasViewportContext(): CanvasViewportContext {
  if (!hasInjectionContext()) return defaultCanvasViewportContext
  return inject(CANVAS_VIEWPORT_CONTEXT_KEY, defaultCanvasViewportContext)
}
