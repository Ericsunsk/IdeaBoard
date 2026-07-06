import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
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
  type ImageInpaintExecutionPlan,
  type ImagePromptExecutionPlan,
  type TextPromptExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { composeFlowLocalInpaintTaskResult } from '../result-nodes/flowLocalInpaintTaskResult'
import {
  assertTaskRequestReady,
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from '../task-runner/flowImageDirectActionExecutor'
import {
  useImageExecutionLifecycle,
  type ImageExecutionLifecycle,
} from './useImageExecutionLifecycle'

type TaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
}

type FlowImageWorkflowLifecycle = Pick<
  ImageExecutionLifecycle,
  'resolveFirstTaskImageResult' | 'runImageExecutionBatch'
>

export type FlowImageWorkflowExecutionPlan =
  | ImageActionExecutionPlan
  | CanvasExtendExecutionPlan
  | ImageInpaintExecutionPlan
  | ImagePromptExecutionPlan
  | TextPromptExecutionPlan
  | FusionExecutionPlan

type FlowImageWorkflowExecutionOptions = {
  lifecycle: FlowImageWorkflowLifecycle
  getCurrentBoardId?: () => string
}

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore?: () => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
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
    await options.lifecycle.runImageExecutionBatch({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      boardId,
      outputCount: 1,
      assertReady: taskSubmission.assertReady,
      submitTask: () => taskSubmission.submitTask(),
      resolveResult: (taskId) =>
        options.lifecycle.resolveFirstTaskImageResult(taskId, plan.source.ratio),
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
    await options.lifecycle.runImageExecutionBatch({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) =>
        options.lifecycle.resolveFirstTaskImageResult(taskId, plan.targetRatio),
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
    await options.lifecycle.runImageExecutionBatch({
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
    await options.lifecycle.runImageExecutionBatch({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) =>
        options.lifecycle.resolveFirstTaskImageResult(taskId, plan.source.ratio),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function executeTextPrompt(plan: TextPromptExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => buildImageExecutionTaskRequest(plan),
    })
    await options.lifecycle.runImageExecutionBatch({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) => options.lifecycle.resolveFirstTaskImageResult(taskId, 1),
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
    await options.lifecycle.runImageExecutionBatch({
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
      resolveResult: (taskId) => options.lifecycle.resolveFirstTaskImageResult(taskId, targetRatio),
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
  const lifecycle = useImageExecutionLifecycle({
    nodes: options.nodes,
    edges: options.edges,
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    refreshNodeInternals: options.refreshNodeInternals,
    flushAutosave: options.flushAutosave,
    getCurrentBoardId: options.getCurrentBoardId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })
  const executor = createFlowImageWorkflowExecutor({
    lifecycle,
    getCurrentBoardId: options.getCurrentBoardId,
  })

  return {
    executeImageWorkflow: executor.executeImageWorkflow,
    isNodeDirectActionRunning: lifecycle.isNodeDirectActionRunning,
    reattachPendingImageResults: lifecycle.reattachPendingImageResults,
  }
}
