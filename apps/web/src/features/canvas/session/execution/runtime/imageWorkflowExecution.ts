import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { CanvasGraphStore } from '../../../core/graph'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import { EXECUTION_SERVICE } from '../../../core/execution/executionContract'
import {
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
} from '../commands/imageTaskRequests'
import { prepareInpaintMaskAsset } from '../media/inpaintMask'
import { readImageNaturalSize } from '../../../../../core/media/imageDimensions'
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
import type { TaskImageOutcomeWaiter } from '../task-runner/taskImageOutcome'
import { type CanvasExecutionWorkspace } from '../workspace/useCanvasExecutionWorkspace'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'
import { assemblePendingImageWorkflow } from './pendingWorkflowAssembly'
import {
  getMediaAssetMetadata,
  resolveMediaAssetProcessingURL,
} from '@/api/media/mediaAssetDelivery'

export type ImageWorkflowExecutionPlan = ActivatedImageExecutionPlan

export type UseImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  boardBaseSnapshot: Ref<BoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { isSilent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
  executionWorkspace: CanvasExecutionWorkspace
  cancelTasks: (taskIds: readonly string[]) => void
  taskTransport?: PendingImageWorkflowTaskTransport
  /** Shared task-result waiter. Defaults to the canonical waiter so every
   * execution path reconciles completion through the same seam instead of
   * fragmenting wait logic per workflow entrypoint (issue #676). */
  waitTaskImageOutcome?: TaskImageOutcomeWaiter
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
    waitTaskImageOutcome: options.waitTaskImageOutcome,
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
    const imageSize =
      plan.action === EXECUTION_SERVICE.variations && source.assetId
        ? await getMediaAssetMetadata(source.assetId)
        : await readImageNaturalSize(source.url)
    const taskRequest = buildImageExecutionTaskRequest({ ...plan, source }, imageSize)
    const resultSource =
      plan.action === EXECUTION_SERVICE.variations && imageSize
        ? { ...source, ratio: imageSize.height / imageSize.width }
        : source
    await workflowRunner.execute({
      source: resultSource,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      attachment,
      outputCount: plan.outputCount,
      taskRequest,
      fallbackRatio: resultSource.ratio,
    })
  }

  async function executeOutpaint(
    plan: OutpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const source = await resolveExecutionSource(plan.source)
    const taskRequest = buildImageExecutionTaskRequest(
      { ...plan, source },
      await readImageNaturalSize(source.url),
    )
    await workflowRunner.execute({
      source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
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
    // the direct-upload flow first, then submits with mask_asset_id.
    const [maskAssetId, imageSize] = await Promise.all([
      prepareInpaintMaskAsset(plan.rawMaskDataUrl, workspaceId),
      readImageNaturalSize(source.url),
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
      source: await resolveExecutionSource(plan.source),
    }
    const imageSizes = await Promise.all(
      resolvedInputs.map((input) => readImageNaturalSize(input.url)),
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
