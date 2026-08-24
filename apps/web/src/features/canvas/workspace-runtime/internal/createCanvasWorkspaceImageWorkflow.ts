import type { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import type { CanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import { useCanvasImageWorkflow, type CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import type { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import type { PendingImageExecutionCore } from '../../execution/pending-execution'

type CreateCanvasWorkspaceImageWorkflowOptions = {
  board: ReturnType<typeof useCanvasBoardRuntime>
  graph: ReturnType<typeof createCanvasWorkspaceGraphRuntime>
  flowPaneAdapter: CanvasFlowPaneAdapter
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  ui: CanvasImageWorkflowUiAdapter
  feedback: Pick<CanvasFeedback, 'imageTools' | 'taskExecution'>
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
}

export function createCanvasWorkspaceImageWorkflow(
  options: CreateCanvasWorkspaceImageWorkflowOptions,
) {
  return useCanvasImageWorkflow({
    graph: options.graph.imageWorkflowPort,
    execution: {
      edges: options.board.flowPane.edges,
      readCanvasGraphStore: options.board.graphEditing.readCanvasGraphStore,
      boardBaseSnapshot: options.board.boardSession.boardBaseSnapshot,
      flushAutosave: options.flushAutosave,
      refreshNodeInternals: options.graph.executionPort.refreshNodeInternals,
      executionCore: options.executionCore,
      cancelTasks: options.cancelTasks,
    },
    flowPaneAdapter: options.flowPaneAdapter,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.ui,
    feedback: options.feedback,
  })
}
