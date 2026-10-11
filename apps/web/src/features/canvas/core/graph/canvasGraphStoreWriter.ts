import type { CanvasGraphReadPort } from './canvasGraphStoreTypes'
import type { CanvasGraphStore } from './canvasGraphStore'

/** Resolve the mutable graph port used by application commands. */
export function resolveWritableCanvasGraphStore(
  readCanvasGraphStore: () => CanvasGraphReadPort,
  writeCanvasGraphStore?: () => CanvasGraphStore,
): CanvasGraphStore {
  return writeCanvasGraphStore?.() ?? (readCanvasGraphStore() as CanvasGraphStore)
}
