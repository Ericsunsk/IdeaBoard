import type { Edge, Node } from '@vue-flow/core'
import { getCurrentScope, onScopeDispose, type Ref } from 'vue'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import type { TaskImageOutcomeWaiter } from '../task-runner/taskImageOutcome'
import type { CanvasGraphStore } from '../../../core/graph'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import { readImageNaturalSize } from '../../../../../core/media/imageDimensions'
import type { ActivatedImageExecutionPlan } from '../planning/imageExecutionPlan'
import { type ImageSizeLoader } from '../result-nodes/resultDisplay'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultExecution.ts'
import { waitCanvasWorkspaceTaskImageOutcome } from './canvasTaskWaiting'
import type { ClientActionIdFactory } from '../task-runner/taskClientActionIds'
import { buildImageExecutionTaskRequest } from '../commands/imageTaskRequests'
import type { PendingImageExecutionCore } from '../pending-execution/core'
import { createCanvasExecutionRecoveryCoordinator } from '../application/canvasExecutionRecoveryCoordinator'
import { assemblePendingImageWorkflow } from '../runtime/pendingWorkflowAssembly'
import { createTaskImageObserver } from '../task-runner/taskImageOutcome'
import { resolveMediaAssetProcessingURL } from '@/api/media/mediaAssetDelivery'

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
  waitTaskImageOutcome?: TaskImageOutcomeWaiter
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
  // 单层默认：调用方在此显式解析后透传给装配（装配仅保留直接调用方的回退），
  // 不再与装配内各持一份 `??` 默认。
  const waitTaskImageOutcome = options.waitTaskImageOutcome ?? waitCanvasWorkspaceTaskImageOutcome
  const loadImageSize = options.loadImageSize ?? readImageNaturalSize
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
    waitTaskImageOutcome,
    loadImageSize,
    createClientActionId: options.createClientActionId,
    cancelTasks: options.cancelTasks,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeRunning(nodeId: string): boolean {
    return options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0
  }

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
      fallbackRatio: source.ratio,
    })
  }

  const recovery = createCanvasExecutionRecoveryCoordinator({
    executionCore: options.executionCore,
    readBoardSnapshot: () => options.boardBaseSnapshot.value,
    observeTask: createTaskImageObserver(waitTaskImageOutcome, loadImageSize),
    flushAutosave: flushPendingTaskSnapshot,
    cancelTasks: options.cancelTasks,
    onError: options.onError,
    onWarning: options.onWarning,
  })

  if (getCurrentScope()) onScopeDispose(recovery.dispose)

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
