import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import type { TaskImageResult } from '../../../../../core/query/taskResult'
import type { CanvasGraphStore } from '../../../core/graph'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import { loadImageSizeFromUrl } from '../media/imageSize'
import type { ActivatedImageExecutionPlan } from '../planning/imageExecutionPlan'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageResultAtIndexDisplay,
  type ImageSizeLoader,
} from '../result-nodes/resultDisplay'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultExecution.ts'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/imageDirectActionExecutor'
import type { ClientActionIdFactory } from '../task-runner/taskClientActionIds'
import { buildImageExecutionTaskRequest } from '../commands/imageTaskRequests'
import type { PendingImageExecutionCore } from '../pending-execution/core'
import { createCanvasExecutionRecoveryCoordinator } from '../application/canvasExecutionRecoveryCoordinator'
import { type CanvasTaskSubmissionAdapter } from './canvasTaskSubmissionAdapter'
import { assemblePendingImageWorkflow } from '../runtime/pendingWorkflowAssembly'
import { resolveMediaAssetProcessingURL } from '../../../../../api/mediaAssetDelivery'

type CanvasExecutionWorkspaceState = {
  readonly executionCore: PendingImageExecutionCore
  isNodeRunning: (nodeId: string) => boolean
}

type CanvasExecutionWorkspaceCommands = {
  runGeneration: (plan: ActivatedImageExecutionPlan & { kind: 'generation' }) => Promise<void>
  hydratePendingImageLeases: (snapshot?: BoardSnapshot) => number
  reattachPendingImageResults: () => Promise<void>
}

export type CanvasExecutionWorkspace = {
  state: CanvasExecutionWorkspaceState
  commands: CanvasExecutionWorkspaceCommands
}

export type UseCanvasExecutionWorkspaceOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  boardBaseSnapshot: Ref<BoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { isSilent?: boolean }) => Promise<void>
  pendingResults?: PendingImageResultExecution
  taskAdapter?: CanvasTaskSubmissionAdapter
  waitTaskImageResults?: (taskId: string) => Promise<TaskImageResult[]>
  loadImageSize?: ImageSizeLoader
  createClientActionId?: ClientActionIdFactory
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
}

export function useCanvasExecutionWorkspace(
  options: UseCanvasExecutionWorkspaceOptions,
): CanvasExecutionWorkspace {
  const { workflowRunner, flushPendingTaskSnapshot } = assemblePendingImageWorkflow({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    rendererMetrics: options.rendererMetrics,
    refreshNodeInternals: options.refreshNodeInternals,
    flushAutosave: options.flushAutosave,
    pendingResults: options.pendingResults,
    executionCore: options.executionCore,
    isNodeExecutionActive: (nodeId) =>
      options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0,
    taskAdapter: options.taskAdapter,
    waitTaskImageResults: options.waitTaskImageResults,
    loadImageSize: options.loadImageSize,
    createClientActionId: options.createClientActionId,
    cancelTasks: options.cancelTasks,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeRunning(nodeId: string): boolean {
    return options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0
  }

  const waitTaskImageResults = options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults
  const loadImageSize = options.loadImageSize ?? loadImageSizeFromUrl

  function requireAttachment() {
    const attachment = options.executionCore.currentAttachment()
    if (!attachment) throw new Error('Canvas execution requires an active Board attachment')
    return attachment
  }

  async function runGeneration(
    plan: ActivatedImageExecutionPlan & { kind: 'generation' },
  ): Promise<void> {
    const attachment = requireAttachment()
    const resolvedReferences = await Promise.all(
      plan.referenceImages.map(async (image) =>
        image.assetId
          ? { ...image, url: await resolveMediaAssetProcessingURL(image.assetId) }
          : image,
      ),
    )
    const source = plan.source.assetId
      ? { ...plan.source, url: await resolveMediaAssetProcessingURL(plan.source.assetId) }
      : plan.source
    const resolvedPlan = { ...plan, source, referenceImages: resolvedReferences }
    const taskRequest = buildImageExecutionTaskRequest(resolvedPlan)
    await workflowRunner.execute({
      source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: source.ratio,
    })
  }

  const recovery = createCanvasExecutionRecoveryCoordinator({
    executionCore: options.executionCore,
    readBoardSnapshot: () => options.boardBaseSnapshot.value,
    resolveResult: async ({ taskId, resultIndex, fallbackRatio }) => {
      const results = await waitTaskImageResults(taskId)
      return resultIndex === undefined
        ? await resolveFirstTaskImageResultDisplay(results, fallbackRatio, loadImageSize)
        : await resolveTaskImageResultAtIndexDisplay(
            results,
            resultIndex,
            fallbackRatio,
            loadImageSize,
          )
    },
    flushAutosave: flushPendingTaskSnapshot,
    cancelTasks: options.cancelTasks,
    onError: options.onError,
    onWarning: options.onWarning,
  })

  return {
    state: { executionCore: options.executionCore, isNodeRunning },
    commands: {
      runGeneration,
      hydratePendingImageLeases: recovery.hydratePendingLeases,
      reattachPendingImageResults: async () => {
        await recovery.recover()
      },
    },
  }
}
