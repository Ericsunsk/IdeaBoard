import type { useCanvasBoardRuntime } from './useCanvasBoardRuntime'
import type { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import type { useCanvasViewportRuntime } from '../../vue-flow-adapter/useCanvasViewportRuntime'
import type { createCanvasWorkspaceImageWorkflow } from './createCanvasWorkspaceImageWorkflow'
import type { createCanvasWorkspaceAutosave } from './createCanvasWorkspaceAutosave'

export type CanvasWorkspaceRuntimeParts = {
  board: ReturnType<typeof useCanvasBoardRuntime>
  graph: ReturnType<typeof createCanvasWorkspaceGraphRuntime>
  viewport: ReturnType<typeof useCanvasViewportRuntime>
  persistence: {
    flushAutosave: ReturnType<typeof createCanvasWorkspaceAutosave>
    history: { undo: () => void; redo: () => void }
    recoverBoardSession: () => Promise<void>
  }
  imageWorkflow: ReturnType<typeof createCanvasWorkspaceImageWorkflow>
}
