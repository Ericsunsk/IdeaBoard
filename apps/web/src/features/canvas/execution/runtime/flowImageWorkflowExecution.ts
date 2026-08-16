import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { FlowToCanvasBoardSnapshotOptions } from '../../utils/flowSnapshotAdapter'
import {
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
} from '../commands/flowImageTaskRequests'
import { prepareFlowPartialEditMask } from '../media/flowPartialEditMask'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  prepareFusionExecutionPlan,
  type ActivatedImageExecutionPlan,
  type CanvasExtendExecutionPlan,
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
import type { PendingImageAttachment } from '../pending-execution'
import { createCanvasTaskClientActionId, withTaskClientActionId } from '../task-runner/taskClientActionIds'

export type FlowImageWorkflowExecutionPlan = ActivatedImageExecutionPlan

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => CanvasGraphStore
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
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    refreshNodeInternals: options.refreshNodeInternals,
    isNodeExecutionActive: options.executionWorkspace.state.isNodeRunning,
  })

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning('画布保存失败，刷新页面后任务进度可能丢失')
    }
  }

  const executionWorkspace = options.executionWorkspace
  const executionCore = executionWorkspace.state.executionCore
  const taskTransport = options.taskTransport ?? {
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
  } satisfies PendingImageWorkflowTaskTransport

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

  function requireCurrentAttachment(): PendingImageAttachment {
    const attachment = executionCore.currentAttachment()
    if (!attachment) {
      throw new Error('Canvas execution requires the current Board attachment')
    }
    return attachment
  }

  async function executeImageAction(
    plan: ImageActionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const taskRequest = buildImageExecutionTaskRequest(
      plan,
      await loadImageSizeFromUrl(plan.source.url),
    )
    await workflowRunner.execute({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      attachment,
      outputCount: plan.outputCount,
      taskRequest,
      readinessRequest: buildImageActionReadinessRequest(plan.action),
      fallbackRatio: plan.source.ratio,
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
  }

  async function executeCanvasExtend(
    plan: CanvasExtendExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const taskRequest = buildImageExecutionTaskRequest(
      plan,
      await loadImageSizeFromUrl(plan.source.url),
    )
    await workflowRunner.execute({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: plan.targetRatio,
      fallbackErrorMessage: '扩图失败',
    })
  }

  async function executeImageInpaint(
    plan: ImageInpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const boardId = attachment.boardId
    const [maskImageUrl, imageSize] = await Promise.all([
      prepareFlowPartialEditMask(plan.rawMaskDataUrl, boardId),
      loadImageSizeFromUrl(plan.source.url),
    ])
    const taskRequest = buildPreparedImageInpaintTaskRequest({ plan, maskImageUrl, imageSize })
    await workflowRunner.execute({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: plan.source.ratio,
      fallbackErrorMessage: '局部重绘失败',
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
    const imageSizes = await Promise.all(plan.inputImages.map((input) => loadImageSizeFromUrl(input.url)))
    const preparedPlan = prepareFusionExecutionPlan(plan, imageSizes)
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
      fallbackErrorMessage: 'Fusion 生成失败',
      formatErrorMessage: (error) => sanitizeApiErrorMessage(error.message, 'Fusion 生成失败'),
    })
  }

  async function executeImageWorkflow(plan: FlowImageWorkflowExecutionPlan): Promise<void> {
    switch (plan.kind) {
      case 'image-action':
        return await executeImageAction(plan)
      case 'canvas-extend':
        return await executeCanvasExtend(plan)
      case 'image-inpaint':
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
    reattachPendingImageResults: executionWorkspace.commands.reattachPendingImageResults,
  }
}
