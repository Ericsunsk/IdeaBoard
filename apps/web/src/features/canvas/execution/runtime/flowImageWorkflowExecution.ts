import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { FlowToCanvasBoardSnapshotOptions } from '../../utils/flowSnapshotAdapter'
import {
  buildCanvasExtendReadinessRequest,
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildLocalInpaintReadinessRequest,
  buildPreparedImageInpaintTaskRequest,
  buildPromptImageReadinessRequest,
} from '../commands/flowImageTaskRequests'
import { prepareFlowLocalInpaintMask } from '../media/flowLocalInpaintMask'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  prepareFusionExecutionPlan,
  type ActivatedImageExecutionPlan,
  type CanvasExtendExecutionPlan,
  type FusionExecutionPlan,
  type ImageActionExecutionPlan,
  type ImageInpaintExecutionPlan,
  type ImagePromptExecutionPlan,
  type PromptGenerationExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { resolveTaskImageResultAtIndexDisplay } from '../result-nodes/flowImageDirectActionTaskResult'
import { usePendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import { createCanvasTaskSubmissionCallbacks } from '../workspace/canvasTaskSubmissionAdapter'
import { type CanvasExecutionWorkspace } from '../workspace/useCanvasExecutionWorkspace'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'
import type { CanvasExecutionAttachment } from './CanvasExecutionRuntime'

export type FlowImageWorkflowExecutionPlan = ActivatedImageExecutionPlan

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
  executionWorkspace: CanvasExecutionWorkspace
  cancelTasks: (taskIds: readonly string[]) => void
}

function requirePreparedTaskRequest(
  request: SubmitTaskData | null,
  fallbackMessage: string,
): SubmitTaskData {
  if (!request) throw new Error(fallbackMessage)
  return request
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

  function createIndexedTaskResultResolver(fallbackRatio: () => number) {
    let taskResultsPromise: ReturnType<typeof waitExistingOrLiveTaskImageResults> | null = null
    return async (taskId: string, _resultNodeId: string, resultIndex: number) => {
      taskResultsPromise ??= waitExistingOrLiveTaskImageResults(taskId)
      return await resolveTaskImageResultAtIndexDisplay(
        await taskResultsPromise,
        resultIndex,
        fallbackRatio(),
        loadImageSizeFromUrl,
      )
    }
  }

  const executionWorkspace = options.executionWorkspace
  const executionRuntime = executionWorkspace.state.executionRuntime

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionRuntime,
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeExecutionActive(nodeId: string): boolean {
    return executionWorkspace.state.isNodeRunning(nodeId)
  }

  function requireCurrentAttachment(): CanvasExecutionAttachment {
    const attachment = executionRuntime.attachment.value
    if (!attachment) {
      throw new Error('Canvas execution requires the current Board attachment')
    }
    return attachment
  }

  async function executeImageAction(
    plan: ImageActionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => buildImageActionReadinessRequest(plan.action),
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      attachment,
      outputCount: plan.outputCount,
      submissionMode: 'shared-task',
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: createIndexedTaskResultResolver(() => plan.source.ratio),
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
  }

  async function executeCanvasExtend(
    plan: CanvasExtendExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildCanvasExtendReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      submissionMode: 'shared-task',
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: createIndexedTaskResultResolver(() => plan.targetRatio),
      fallbackErrorMessage: '扩图失败',
    })
  }

  async function executeImageInpaint(
    plan: ImageInpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    let request: SubmitTaskData | null = null
    const attachment = requireCurrentAttachment()
    const boardId = attachment.boardId
    const readinessSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildLocalInpaintReadinessRequest,
      buildRequest: buildLocalInpaintReadinessRequest,
    })
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
      fallbackMessage: '提交局部重绘任务失败',
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      submissionMode: 'shared-task',
      resultOptions: plan.resultOptions,
      preflight: readinessSubmission.assertReady,
      prepare: async () => {
        const maskImageUrl = await prepareFlowLocalInpaintMask(plan.rawMaskDataUrl, boardId)
        request = buildPreparedImageInpaintTaskRequest({
          plan,
          maskImageUrl,
        })
      },
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: createIndexedTaskResultResolver(() => plan.source.ratio),
      fallbackErrorMessage: '局部重绘失败',
    })
  }

  async function executeImagePrompt(
    plan: ImagePromptExecutionPlan & { activationId: string },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      submissionMode: 'shared-task',
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: createIndexedTaskResultResolver(() => plan.source.ratio),
      fallbackErrorMessage: '生成失败',
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
    let targetRatio = plan.source.ratio
    let request: SubmitTaskData | null = null
    let taskResultsPromise: ReturnType<typeof waitExistingOrLiveTaskImageResults> | null = null
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      buildRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      fallbackMessage: '提交 Fusion 任务失败',
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      submissionMode: 'shared-task',
      prepare: async () => {
        const imageSizes = await Promise.all(
          plan.inputImages.map((input) => loadImageSizeFromUrl(input.url)),
        )
        const preparedPlan = prepareFusionExecutionPlan(plan, imageSizes)
        targetRatio = preparedPlan.targetRatio
        request = buildImageExecutionTaskRequest(preparedPlan)
        return {
          source: preparedPlan.source,
          resultOptions: preparedPlan.resultOptions,
        }
      },
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: async (taskId, _resultNodeId, resultIndex) => {
        taskResultsPromise ??= waitExistingOrLiveTaskImageResults(taskId)
        return resolveTaskImageResultAtIndexDisplay(
          await taskResultsPromise,
          resultIndex,
          targetRatio,
          loadImageSizeFromUrl,
        )
      },
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
      case 'image-prompt':
        return await executeImagePrompt(plan)
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
