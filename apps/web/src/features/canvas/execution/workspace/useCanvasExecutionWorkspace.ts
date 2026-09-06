import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { TaskImageResult } from '../../../../shared/query/taskResult'
import { i18n } from '../../../../shared/i18n'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasRendererMetricsPort } from '../../vue-flow-adapter'
import { loadImageSizeFromUrl } from '../media/imageSize'
import type { ActivatedImageExecutionPlan } from '../planning/ImageExecutionPlan'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageResultAtIndexDisplay,
  type ImageSizeLoader,
} from '../result-nodes/flowImageDirectActionTaskResult'
import {
  type PendingImageResultExecution,
  usePendingImageResultExecution,
} from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import { buildImageExecutionTaskRequest } from '../commands/flowImageTaskRequests'
import { createPendingImageWorkflowRunner } from '../runtime/pendingImageWorkflowRunner'
import type { PendingImageExecutionCore } from '../pending-execution'
import { createCanvasExecutionRecoveryCoordinator } from '../application/canvasExecutionRecoveryCoordinator'
import {
  defaultCanvasTaskSubmissionAdapter,
  type CanvasTaskSubmissionAdapter,
} from './canvasTaskSubmissionAdapter'
import { resolveMediaAssetProcessingURL } from '../../../../shared/api/mediaAssetDelivery'

export type CanvasExecutionWorkspaceState = {
  readonly executionCore: PendingImageExecutionCore
  isNodeRunning: (nodeId: string) => boolean
}

export type CanvasExecutionWorkspaceCommands = {
  runGeneration: (plan: ActivatedImageExecutionPlan & { kind: 'generation' }) => Promise<void>
  hydratePendingImageLeases: (snapshot?: CanvasBoardSnapshot) => number
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
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
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
  const waitTaskImageResults = options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults
  const loadImageSize = options.loadImageSize ?? loadImageSizeFromUrl
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const pendingResults =
    options.pendingResults ??
    usePendingImageResultExecution({
      nodes: options.nodes,
      edges: options.edges,
      readCanvasGraphStore: options.readCanvasGraphStore,
      writeCanvasGraphStore: options.readCanvasGraphStore,
      rendererMetrics: options.rendererMetrics,
      readCurrentAttachment: options.executionCore.currentAttachment,
      isAttachmentCurrent: options.executionCore.isAttachmentCurrent,
      refreshNodeInternals: options.refreshNodeInternals,
      isNodeExecutionActive: (nodeId) =>
        options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0,
    })

  function isNodeRunning(nodeId: string): boolean {
    return options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0
  }

  function requireAttachment() {
    const attachment = options.executionCore.currentAttachment()
    if (!attachment) throw new Error('Canvas execution requires an active Board attachment')
    return attachment
  }

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning(i18n.global.t('canvas.execution.canvasSaveFailedProgressLost'))
    }
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionCore: options.executionCore,
    taskTransport: {
      assertReady: (request, fallbackMessage) =>
        taskAdapter.assertTaskRequestReady(
          withTaskClientActionId(request, createCanvasTaskClientActionId()),
          fallbackMessage,
        ),
      submit: (request, clientActionId, fallbackMessage) =>
        taskAdapter.submitTaskRequest(
          withTaskClientActionId(request, clientActionId),
          fallbackMessage,
        ),
      resolve: async (taskId, resultIndex, fallbackRatio) =>
        resolveTaskImageResultAtIndexDisplay(
          await waitTaskImageResults(taskId),
          resultIndex,
          fallbackRatio,
          loadImageSize,
        ),
    },
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    createClientActionId: options.createClientActionId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  async function runGeneration(
    plan: ActivatedImageExecutionPlan & { kind: 'generation' },
  ): Promise<void> {
    const attachment = requireAttachment()
    const resolvedReferences = await Promise.all(
      plan.referenceImages.map(async (image) =>
        image.mediaAssetId
          ? { ...image, url: await resolveMediaAssetProcessingURL(image.mediaAssetId) }
          : image,
      ),
    )
    const source = plan.source.mediaAssetId
      ? { ...plan.source, url: await resolveMediaAssetProcessingURL(plan.source.mediaAssetId) }
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
