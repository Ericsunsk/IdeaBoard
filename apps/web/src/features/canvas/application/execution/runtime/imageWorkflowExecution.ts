import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { CanvasGraphStore } from '../../../domain/graph'
import type { CanvasRendererMetricsPort } from '../../../infrastructure/vue-flow-adapter'
import type { BoardSnapshot } from '../../../../board'
import {
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
} from '../commands/imageTaskRequests'
import { prepareInpaintMaskAsset } from '../media/inpaintMask'
import { loadImageSizeFromUrl } from '../media/imageSize'
import { requireCanvasWorkspaceId } from '../workspace/canvasWorkspaceTaskAdapter'
import {
  prepareFusionExecutionPlan,
  type ActivatedImageExecutionPlan,
  type OutpaintExecutionPlan,
  type FusionExecutionPlan,
  type ImageActionExecutionPlan,
  type ImageInpaintExecutionPlan,
  type GenerationExecutionPlan,
} from '../planning/imageExecutionPlan'
import type { BoardAttachment } from '../board-attachment'
import type { TaskImageResult } from '../../../../../shared/query/taskResult'
import { type CanvasExecutionWorkspace } from '../workspace/useCanvasExecutionWorkspace'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'
import { assemblePendingImageWorkflow } from './pendingWorkflowAssembly'
import { resolveMediaAssetProcessingURL } from '../../../../../shared/api/mediaAssetDelivery'

export type ImageWorkflowExecutionPlan = ActivatedImageExecutionPlan

export type UseImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  boardBaseSnapshot: Ref<BoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
  executionWorkspace: CanvasExecutionWorkspace
  cancelTasks: (taskIds: readonly string[]) => void
  taskTransport?: PendingImageWorkflowTaskTransport
  /** Shared task-result waiter. Defaults to the canonical waiter so every
   * execution path reconciles completion through the same seam instead of
   * fragmenting wait logic per workflow entrypoint (issue #676). */
  waitTaskImageResults?: (taskId: string) => Promise<TaskImageResult[]>
}

export function useImageWorkflowExecution(options: UseImageWorkflowExecutionOptions) {
  const executionWorkspace = options.executionWorkspace
  const executionCore = executionWorkspace.state.executionCore
  const { workflowRunner } = assemblePendingImageWorkflow({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    rendererMetrics: options.rendererMetrics,
    refreshNodeInternals: options.refreshNodeInternals,
    flushAutosave: options.flushAutosave,
    executionCore,
    isNodeExecutionActive: executionWorkspace.state.isNodeRunning,
    taskTransport: options.taskTransport,
    waitTaskImageResults: options.waitTaskImageResults,
    cancelTasks: options.cancelTasks,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeExecutionActive(nodeId: string): boolean {
    return executionWorkspace.state.isNodeRunning(nodeId)
  }

  function requireCurrentAttachment(): BoardAttachment {
    const attachment = executionCore.currentAttachment()
    if (!attachment) {
      throw new Error('Canvas execution requires the current Board attachment')
    }
    return attachment
  }

  async function resolveExecutionSource<T extends { url: string; assetId?: string | undefined }>(
    source: T,
  ): Promise<T> {
    if (!source.assetId) return source
    const url = await resolveMediaAssetProcessingURL(source.assetId)
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
    const workspaceId = requireCanvasWorkspaceId()
    const source = await resolveExecutionSource(plan.source)
    // v1 inpaint links the mask by asset id: the painted data-URL travels
    // the upload-session flow first, then submits with mask_asset_id.
    const [maskAssetId, imageSize] = await Promise.all([
      prepareInpaintMaskAsset(plan.rawMaskDataUrl, workspaceId),
      loadImageSizeFromUrl(source.url),
    ])
    const taskRequest = buildPreparedImageInpaintTaskRequest({
      plan: { ...plan, source },
      maskAssetId,
      imageSize,
    })
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

  async function executeGeneration(
    plan: GenerationExecutionPlan & { activationId: string },
  ): Promise<void> {
    await executionWorkspace.commands.runGeneration(plan)
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

  async function executeImageWorkflow(plan: ImageWorkflowExecutionPlan): Promise<void> {
    switch (plan.kind) {
      case 'image-action':
        return await executeImageAction(plan)
      case 'outpaint':
        return await executeOutpaint(plan)
      case 'inpaint':
        return await executeImageInpaint(plan)
      case 'generation':
        return await executeGeneration(plan)
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
