import {
  hasInjectionContext,
  inject,
  provide,
  readonly,
  ref,
  type InjectionKey,
  type Ref,
} from 'vue'

import type { CanvasImageQuality } from './useCanvasImageQualityZoom'

export const DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL = 100

export function resolveCanvasNodeUiScale(zoomLevel: number): number {
  const normalizedZoom = Number(zoomLevel) / DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL
  return Number.isFinite(normalizedZoom) && normalizedZoom > 0 ? 1 / normalizedZoom : 1
}

export type CanvasViewportContext = {
  zoomLevel: Readonly<Ref<number>>
  imageQuality: CanvasImageQuality
}

const CANVAS_VIEWPORT_CONTEXT_KEY: InjectionKey<CanvasViewportContext> = Symbol('canvas:viewport')

const defaultCanvasViewportContext: CanvasViewportContext = {
  zoomLevel: readonly(ref(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)),
  imageQuality: {
    zoomLevel: readonly(ref(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)),
    isSettled: readonly(ref(true)),
    requestSettle: () => {},
  },
}

export function provideCanvasViewportContext(context: CanvasViewportContext) {
  provide(CANVAS_VIEWPORT_CONTEXT_KEY, {
    zoomLevel: readonly(context.zoomLevel),
    imageQuality: {
      zoomLevel: readonly(context.imageQuality.zoomLevel),
      isSettled: readonly(context.imageQuality.isSettled),
      requestSettle: context.imageQuality.requestSettle,
    },
  })
}

export function useCanvasViewportContext(): CanvasViewportContext {
  if (!hasInjectionContext()) return defaultCanvasViewportContext
  return inject(CANVAS_VIEWPORT_CONTEXT_KEY, defaultCanvasViewportContext)
}
