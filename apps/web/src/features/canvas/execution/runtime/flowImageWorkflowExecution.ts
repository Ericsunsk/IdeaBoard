import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { FlowTransactions } from '../../composables/useFlowTransactions'
import { readCurrentBoardId } from '../../utils/currentBoardId'
import {
  buildCanvasExtendReadinessRequest,
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
  buildPromptImageReadinessRequest,
} from '../commands/flowImageTaskRequests'
import {
  cleanupFlowLocalInpaintMedia,
  prepareFlowLocalInpaintMedia,
  type PreparedFlowLocalInpaintMedia,
} from '../media/flowLocalInpaint'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  prepareFusionExecutionPlan,
  type CanvasExtendExecutionPlan,
  type FusionExecutionPlan,
  type ImageActionExecutionPlan,
  type ImageExecutionInput,
  type ImageExecutionResultOptions,
  type ImageInpaintExecutionPlan,
  type ImagePromptExecutionPlan,
  type TextPromptExecutionPlan,
} from '../planning/ImageExecutionPlan'
import type { ResolvedTaskImageResult } from '../result-nodes/flowImageDirectActionTaskResult'
import { composeFlowLocalInpaintTaskResult } from '../result-nodes/flowLocalInpaintTaskResult'
import { usePendingImageResultNodeLifecycle } from '../result-nodes/usePendingImageResultNodes'
import {
  assertTaskRequestReady,
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from '../task-runner/flowImageDirectActionExecutor'
import { useFlowImageDirectActionRunner } from '../task-runner/useFlowImageDirectActionRunner'

type TaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
}

type PendingImageBatchWorkflowPrepared = {
  source?: ImageExecutionInput
  sourceNodeId?: string
  resultOptions?: ImageExecutionResultOptions
}

type FlowImageWorkflowRunner = {
  resolveFirstTaskImageResult: (
    taskId: string,
    fallbackRatio: number,
  ) => Promise<ResolvedTaskImageResult>
  runPendingImageGeneration: (params: {
    source: ImageExecutionInput
    sourceNodeId: string
    boardId: string
    resultOptions?: ImageExecutionResultOptions
    assertReady?: () => Promise<void>
    submitTask: () => Promise<{ taskId: string }>
    resolveResult: (taskId: string) => Promise<ResolvedTaskImageResult>
    successMessage: string
    fallbackErrorMessage: string
  }) => Promise<void>
  runPendingImageBatchWorkflow: (params: {
    source: ImageExecutionInput
    sourceNodeId: string
    boardId: string
    outputCount: number
    resultOptions?: ImageExecutionResultOptions
    preflight?: () => Promise<void>
    assertReady?: () => Promise<void>
    prepare?: () => Promise<PendingImageBatchWorkflowPrepared | void>
    submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
    resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
    persistPendingTaskIds?: boolean
    cleanup?: () => Promise<void>
    fallbackErrorMessage: string
    formatErrorMessage?: (error: Error) => string
  }) => Promise<void>
}

export type FlowImageWorkflowExecutionPlan =
  | ImageActionExecutionPlan
  | CanvasExtendExecutionPlan
  | ImageInpaintExecutionPlan
  | ImagePromptExecutionPlan
  | TextPromptExecutionPlan
  | FusionExecutionPlan

export type FlowImageWorkflowExecutionOptions = {
  runner: FlowImageWorkflowRunner
  getCurrentBoardId?: () => string
}

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

function createTaskSubmissionCallbacks(options: {
  readinessRequest?: () => SubmitTaskData
  buildRequest: () => SubmitTaskData | Promise<SubmitTaskData>
  fallbackMessage?: string
}): TaskSubmissionCallbacks {
  const { readinessRequest } = options
  const assertReady = readinessRequest
    ? () => assertTaskRequestReady(readinessRequest(), options.fallbackMessage)
    : undefined
  return {
    assertReady,
    submitTask: async () =>
      submitTaskRequest(await options.buildRequest(), options.fallbackMessage),
  }
}

function requirePreparedTaskRequest(
  request: SubmitTaskData | null,
  fallbackMessage: string,
): SubmitTaskData {
  if (!request) throw new Error(fallbackMessage)
  return request
}

function createFlowImageWorkflowExecutor(options: FlowImageWorkflowExecutionOptions) {
  function readBoardId(): string {
    return readCurrentBoardId(options.getCurrentBoardId)
  }

  async function executeImageAction(plan: ImageActionExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: () => buildImageActionReadinessRequest(plan.action),
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await options.runner.runPendingImageGeneration({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      boardId,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) =>
        options.runner.resolveFirstTaskImageResult(taskId, plan.source.ratio),
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
  }

  async function executeCanvasExtend(plan: CanvasExtendExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: buildCanvasExtendReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await options.runner.runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) =>
        options.runner.resolveFirstTaskImageResult(taskId, plan.targetRatio),
      fallbackErrorMessage: '扩图失败',
    })
  }

  async function executeImageInpaint(plan: ImageInpaintExecutionPlan): Promise<void> {
    let localInpaint: PreparedFlowLocalInpaintMedia | null = null
    let request: SubmitTaskData | null = null
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
      fallbackMessage: '提交局部重绘任务失败',
    })
    await options.runner.runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      preflight: () => assertTaskRequestReady(buildPromptImageReadinessRequest()),
      prepare: async () => {
        localInpaint = await prepareFlowLocalInpaintMedia({
          originalImageUrl: plan.source.url,
          rawMaskDataUrl: plan.rawMaskDataUrl,
          boardId,
        })
        request = buildPreparedImageInpaintTaskRequest({
          plan,
          croppedImageUrl: localInpaint.crop.croppedImageUrl,
          croppedMaskUrl: localInpaint.crop.croppedMaskUrl,
          cropSize: localInpaint.crop.cropSize,
        })
      },
      submitTask: taskSubmission.submitTask,
      resolveResult: async (taskId) => {
        if (!localInpaint) throw new Error('局部重绘任务未准备完成')
        return await composeFlowLocalInpaintTaskResult({
          originalImageUrl: plan.source.url,
          croppedTaskResults: await waitExistingOrLiveTaskImageResults(taskId),
          prepared: localInpaint,
          boardId,
          fallbackRatio: plan.source.ratio,
          loadImageSize: loadImageSizeFromUrl,
        })
      },
      cleanup: async () => {
        await cleanupFlowLocalInpaintMedia(localInpaint, boardId)
      },
      persistPendingTaskIds: false,
      fallbackErrorMessage: '局部重绘失败',
      formatErrorMessage: (error) => {
        const message = error.message
        return isTaskResultPendingError(error) ? '局部重绘仍在处理中，请稍后重试' : message
      },
    })
  }

  async function executeImagePrompt(plan: ImagePromptExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await options.runner.runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) =>
        options.runner.resolveFirstTaskImageResult(taskId, plan.source.ratio),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function executeTextPrompt(plan: TextPromptExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => buildImageExecutionTaskRequest(plan),
    })
    await options.runner.runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) => options.runner.resolveFirstTaskImageResult(taskId, 1),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function executeFusion(plan: FusionExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    let targetRatio = plan.source.ratio
    let request: SubmitTaskData | null = null
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      buildRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      fallbackMessage: '提交 Fusion 任务失败',
    })
    await options.runner.runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
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
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) => options.runner.resolveFirstTaskImageResult(taskId, targetRatio),
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
      case 'text-prompt':
        return await executeTextPrompt(plan)
      case 'fusion':
        return await executeFusion(plan)
    }
  }

  return {
    executeImageWorkflow,
  }
}

export function useFlowImageWorkflowExecution(options: UseFlowImageWorkflowExecutionOptions) {
  const pendingImageResultLifecycle = usePendingImageResultNodeLifecycle(options)
  const runner = useFlowImageDirectActionRunner({
    nodes: options.nodes,
    pendingImageResultLifecycle,
    flushAutosave: options.flushAutosave,
    getCurrentBoardId: options.getCurrentBoardId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })
  const executor = createFlowImageWorkflowExecutor({
    runner,
    getCurrentBoardId: options.getCurrentBoardId,
  })

  return {
    executeImageWorkflow: executor.executeImageWorkflow,
    isNodeDirectActionRunning: runner.isNodeDirectActionRunning,
    reattachPendingImageResults: runner.reattachPendingImageResults,
  }
}
