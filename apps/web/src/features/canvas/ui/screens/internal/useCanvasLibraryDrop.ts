import { onScopeDispose, watch, type Ref } from 'vue'
import { createPointerDropTargets } from '../../../../../core/interaction/pointerDropTargets'
import type {
  CanvasLibraryImportDescriptor,
  LibraryDragPayload,
  LibraryDropTargets,
} from '../../../../library'

type Point = { x: number; y: number }

type CanvasLibraryDropOptions = {
  element: Ref<HTMLElement | null>
  context: () => unknown
  canReceive: () => boolean
  screenToFlowCoordinate: (point: Point) => Point
  add: (asset: CanvasLibraryImportDescriptor, point: Point) => unknown
}

export function useCanvasLibraryDrop(options: CanvasLibraryDropOptions): LibraryDropTargets {
  const targets = createPointerDropTargets<LibraryDragPayload>()
  let unregister: (() => void) | undefined
  watch(
    options.element,
    (element) => {
      unregister?.()
      unregister = element
        ? targets.register({
            element,
            canReceive: (payload) => payload.kind === 'asset' && options.canReceive(),
            onHoverChange: () => {},
            onDrop(payload, point) {
              if (payload.kind === 'asset') {
                options.add(payload.asset, options.screenToFlowCoordinate(point))
              }
            },
          })
        : undefined
    },
    { immediate: true, flush: 'sync' },
  )
  watch(options.context, () => targets.cancel(), { flush: 'sync' })
  onScopeDispose(() => {
    targets.cancel()
    unregister?.()
  })
  return targets
}
