import type { useBoardRuntime } from './useBoardRuntime'
import type { CanvasFeedback } from '../useCanvasFeedback.ts'
import type { CanvasVueFlowAdapter } from '../../../session/flow-adapter'
import { useCanvasImageWorkflow, type CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import type { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import type { PendingImageExecutionCore } from '../../execution'

type CreateCanvasWorkspaceImageWorkflowOptions = {
  board: ReturnType<typeof useBoardRuntime>
  graph: ReturnType<typeof createCanvasWorkspaceGraphRuntime>
  flowPaneAdapter: CanvasVueFlowAdapter
  flushAutosave: (options?: { isSilent?: boolean }) => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  ui: CanvasImageWorkflowUiAdapter
  feedback: Pick<CanvasFeedback, 'imageTools' | 'taskExecution'>
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  isWorkspaceAvailable: () => boolean
}

export function createCanvasWorkspaceImageWorkflow(
  options: CreateCanvasWorkspaceImageWorkflowOptions,
) {
  return useCanvasImageWorkflow({
    graph: options.graph.imageWorkflowPort,
    execution: {
      edges: options.board.flowPane.edges,
      readCanvasGraphStore: options.board.graphEditing.readCanvasGraphStore,
      rendererMetrics: options.flowPaneAdapter.rendererMetrics,
      boardBaseSnapshot: options.board.boardSession.boardBaseSnapshot,
      flushAutosave: options.flushAutosave,
      refreshNodeInternals: options.graph.executionPort.refreshNodeInternals,
      executionCore: options.executionCore,
      cancelTasks: options.cancelTasks,
      isWorkspaceAvailable: options.isWorkspaceAvailable,
    },
    flowPaneAdapter: options.flowPaneAdapter,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.ui,
    feedback: options.feedback,
  })
}
