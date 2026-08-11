import type { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import type { CanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import { useCanvasImageWorkflow, type CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import type { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import type { CanvasExecutionRuntime } from '../../execution/runtime/CanvasExecutionRuntime'

type CreateCanvasWorkspaceImageWorkflowOptions = {
  board: ReturnType<typeof useCanvasBoardRuntime>
  graph: ReturnType<typeof createCanvasWorkspaceGraphRuntime>
  flowPaneAdapter: CanvasFlowPaneAdapter
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  ui: CanvasImageWorkflowUiAdapter
  feedback: Pick<CanvasFeedback, 'imageTools' | 'taskExecution'>
  executionRuntime: CanvasExecutionRuntime
}

export function createCanvasWorkspaceImageWorkflow(
  options: CreateCanvasWorkspaceImageWorkflowOptions,
) {
  return useCanvasImageWorkflow({
    graph: options.graph.imageWorkflowPort,
    execution: {
      edges: options.board.flowPane.edges,
      graphAdapter: options.board.graphEditing.adapter,
      createCurrentCanvasGraphStore: options.board.graphEditing.createCurrentCanvasGraphStore,
      boardBaseSnapshot: options.board.boardSession.boardBaseSnapshot,
      flushAutosave: options.flushAutosave,
      refreshNodeInternals: options.graph.executionPort.refreshNodeInternals,
      executionRuntime: options.executionRuntime,
    },
    flowPaneAdapter: options.flowPaneAdapter,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.ui,
    feedback: options.feedback,
  })
}
