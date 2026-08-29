import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasRendererMetricsPort } from '../../vue-flow-adapter'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import { i18n } from '../../../../shared/i18n'
import {
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
} from '../commands/flowImageTaskRequests'
import { prepareFlowInpaintMask } from '../media/flowInpaintMask'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  prepareFusionExecutionPlan,
  type ActivatedImageExecutionPlan,
  type OutpaintExecutionPlan,
  type FusionExecutionPlan,
  type ImageActionExecutionPlan,
  type ImageInpaintExecutionPlan,
  type PromptGenerationExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { resolveTaskImageResultAtIndexDisplay } from '../result-nodes/flowImageDirectActionTaskResult'
import { usePendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import { defaultCanvasTaskSubmissionAdapter } from '../workspace/canvasTaskSubmissionAdapter'
import { type CanvasExecutionWorkspace } from '../workspace/useCanvasExecutionWorkspace'
import {
  createPendingImageWorkflowRunner,
  type PendingImageWorkflowTaskTransport,
} from './pendingImageWorkflowRunner'
import type { CanvasBoardAttachment } from '../../board-attachment'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import { resolveMediaAssetProcessingURL } from '../../../../shared/api/mediaAssetDelivery'

export type FlowImageWorkflowExecutionPlan = ActivatedImageExecutionPlan

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
  executionWorkspace: CanvasExecutionWorkspace
  cancelTasks: (taskIds: readonly string[]) => void
  taskTransport?: PendingImageWorkflowTaskTransport
}

export function useFlowImageWorkflowExecution(options: UseFlowImageWorkflowExecutionOptions) {
  const pendingResults = usePendingImageResultExecution({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    writeCanvasGraphStore: options.readCanvasGraphStore,
    rendererMetrics: options.rendererMetrics,
    readCurrentAttachment: options.executionWorkspace.state.executionCore.currentAttachment,
    isAttachmentCurrent: options.executionWorkspace.state.executionCore.isAttachmentCurrent,
    refreshNodeInternals: options.refreshNodeInternals,
    isNodeExecutionActive: options.executionWorkspace.state.isNodeRunning,
  })

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning(i18n.global.t('canvas.execution.canvasSaveFailedProgressLost'))
    }
  }

  const executionWorkspace = options.executionWorkspace
  const executionCore = executionWorkspace.state.executionCore
  const taskTransport =
    options.taskTransport ??
    ({
      assertReady: (request, fallbackMessage) =>
        defaultCanvasTaskSubmissionAdapter.assertTaskRequestReady(
          withTaskClientActionId(request, createCanvasTaskClientActionId()),
          fallbackMessage,
        ),
      submit: (request, clientActionId, fallbackMessage) =>
        defaultCanvasTaskSubmissionAdapter.submitTaskRequest(
          withTaskClientActionId(request, clientActionId),
          fallbackMessage,
        ),
      resolve: async (taskId, resultIndex, fallbackRatio) =>
        resolveTaskImageResultAtIndexDisplay(
          await waitExistingOrLiveTaskImageResults(taskId),
          resultIndex,
          fallbackRatio,
          loadImageSizeFromUrl,
        ),
    } satisfies PendingImageWorkflowTaskTransport)

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionCore,
    taskTransport,
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeExecutionActive(nodeId: string): boolean {
    return executionWorkspace.state.isNodeRunning(nodeId)
  }

  function requireCurrentAttachment(): CanvasBoardAttachment {
    const attachment = executionCore.currentAttachment()
    if (!attachment) {
      throw new Error('Canvas execution requires the current Board attachment')
    }
    return attachment
  }

  async function resolveExecutionSource<T extends { url: string; mediaAssetId?: number }>(
    source: T,
  ): Promise<T> {
    if (!source.mediaAssetId) return source
    const url = await resolveMediaAssetProcessingURL(source.mediaAssetId)
    return { ...source, url }
  }

  async function executeImageAction(
    plan: ImageActionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const source = await resolveExecutionSource(plan.source)
    const taskRequest = buildImageExecutionTaskRequest(
      { ...plan, source },
      await loadImageSizeFromUrl(source.url),
    )
    await workflowRunner.execute({
      source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      attachment,
      outputCount: plan.outputCount,
      taskRequest,
      readinessRequest: buildImageActionReadinessRequest(plan.action),
      fallbackRatio: source.ratio,
    })
  }

  async function executeOutpaint(
    plan: OutpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const source = await resolveExecutionSource(plan.source)
    const taskRequest = buildImageExecutionTaskRequest(
      { ...plan, source },
      await loadImageSizeFromUrl(source.url),
    )
    await workflowRunner.execute({
      source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: plan.targetRatio,
    })
  }

  async function executeImageInpaint(
    plan: ImageInpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const boardId = attachment.boardId
    const source = await resolveExecutionSource(plan.source)
    const [maskImageUrl, imageSize] = await Promise.all([
      prepareFlowInpaintMask(plan.rawMaskDataUrl, boardId),
      loadImageSizeFromUrl(source.url),
    ])
    const taskRequest = buildPreparedImageInpaintTaskRequest({ plan: { ...plan, source }, maskImageUrl, imageSize })
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

  async function executePromptGeneration(
    plan: PromptGenerationExecutionPlan & { activationId: string },
  ): Promise<void> {
    await executionWorkspace.commands.runPromptGeneration(plan)
  }

  async function executeFusion(
    plan: FusionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const resolvedInputs = await Promise.all(
      plan.inputImages.map(async (input) => resolveExecutionSource(input)),
    )
    const resolvedPlan = {
      ...plan,
      inputImages: resolvedInputs,
      imageUrls: resolvedInputs.map((input) => input.url),
      source: await resolveExecutionSource(plan.source),
    }
    const imageSizes = await Promise.all(
      resolvedInputs.map((input) => loadImageSizeFromUrl(input.url)),
    )
    const preparedPlan = prepareFusionExecutionPlan(resolvedPlan, imageSizes)
    const taskRequest = buildImageExecutionTaskRequest(preparedPlan)
    await workflowRunner.execute({
      source: preparedPlan.source,
      sourceNodeId: preparedPlan.sourceNodeId,
      attachment,
      outputCount: preparedPlan.outputCount,
      resultOptions: preparedPlan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: preparedPlan.targetRatio,
    })
  }

  async function executeImageWorkflow(plan: FlowImageWorkflowExecutionPlan): Promise<void> {
    switch (plan.kind) {
      case 'image-action':
        return await executeImageAction(plan)
      case 'outpaint':
        return await executeOutpaint(plan)
      case 'inpaint':
        return await executeImageInpaint(plan)
      case 'prompt-generation':
        return await executePromptGeneration(plan)
      case 'fusion':
        return await executeFusion(plan)
    }
  }

  return {
    executeImageWorkflow,
    isNodeExecutionActive,
    hydratePendingImageLeases: executionWorkspace.commands.hydratePendingImageLeases,
    reattachPendingImageResults: executionWorkspace.commands.reattachPendingImageResults,
  }
}
