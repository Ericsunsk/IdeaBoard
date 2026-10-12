import type { useBoardRuntime } from './useBoardRuntime'
import type { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import type { useCanvasViewportRuntime } from '../../../session/flow-adapter'
import type { createCanvasWorkspaceImageWorkflow } from './createCanvasWorkspaceImageWorkflow'
import type { createCanvasWorkspaceAutosave } from './createCanvasWorkspaceAutosave'

export type CanvasWorkspaceRuntimeParts = {
  board: ReturnType<typeof useBoardRuntime>
  graph: ReturnType<typeof createCanvasWorkspaceGraphRuntime>
  viewport: ReturnType<typeof useCanvasViewportRuntime>
  persistence: {
    flushAutosave: ReturnType<typeof createCanvasWorkspaceAutosave>
    history: { undo: () => void; redo: () => void }
    recoverBoardSession: () => Promise<void>
  }
  imageWorkflow: ReturnType<typeof createCanvasWorkspaceImageWorkflow>
}
