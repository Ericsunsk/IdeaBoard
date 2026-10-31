import type { CanvasGraphReadPort } from './graphStoreTypes'
import type { CanvasGraphStore } from './graphStore'

/** Resolve the mutable graph port used by application commands. */
export function resolveWritableCanvasGraphStore(
  readCanvasGraphStore: () => CanvasGraphReadPort,
  writeCanvasGraphStore?: () => CanvasGraphStore,
): CanvasGraphStore {
  return writeCanvasGraphStore?.() ?? (readCanvasGraphStore() as CanvasGraphStore)
}
