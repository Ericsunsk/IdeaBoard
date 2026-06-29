import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { FlowTransactions } from '../../composables/useFlowTransactions'
import type { CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import { readCurrentBoardId } from '../../utils/currentBoardId'
import {
  assertTaskRequestReady,
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from '../task-runner/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  cleanupFlowLocalInpaintMedia,
  prepareFlowLocalInpaintMedia,
  type PreparedFlowLocalInpaintMedia,
} from '../media/flowLocalInpaint'
import {
  buildCanvasExtendReadinessRequest,
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
  buildPromptImageReadinessRequest,
} from '../commands/flowImageTaskRequests'
import {
  createImageExecutionPlanner,
  isImageExecutionPlanError,
  prepareFusionExecutionPlan,
  type ImageExecutionInput,
  type ImageResultAnchor,
} from '../planning/ImageExecutionPlan'
import { usePendingImageResultNodes } from '../result-nodes/usePendingImageResultNodes'
import { composeFlowLocalInpaintTaskResult } from '../result-nodes/flowLocalInpaintTaskResult'
import { useFlowImageDirectActionRunner } from '../task-runner/useFlowImageDirectActionRunner'
import type { ImageTaskAction } from '../contracts/imageActionExecution'

type RunImageDirectActionOptions = {
  strength?: number
}

type RunImageInpaintActionOptions = {
  prompt: string
  maskDataUrl: string
  outputCount: number
  referenceImageUrls?: string[]
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

type CanvasExtendRectIntent = {
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
  outputCount?: number
}

type TaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
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

function warnPlanningError(error: unknown, onWarning: (message: string) => void): boolean {
  if (!isImageExecutionPlanError(error)) return false
  onWarning(error.message)
  return true
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const planner = createImageExecutionPlanner(() => ({
    nodes: options.nodes.value,
    edges: options.edges.value,
  }))
  const pendingImageResultNodes = usePendingImageResultNodes(options)
  const {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    resolveFirstTaskImageResult,
    runPendingImageBatchWorkflow,
    runPendingImageGeneration,
  } = useFlowImageDirectActionRunner({
    nodes: options.nodes,
    pendingImageResultNodes,
    flushAutosave: options.flushAutosave,
    getCurrentBoardId: options.getCurrentBoardId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })

  async function runImageDirectAction(
    nodeId: string,
    action: ImageTaskAction,
    runOptions?: RunImageDirectActionOptions,
  ) {
    try {
      const plan = planner.planImageDirectAction(nodeId, action, runOptions)
      const boardId = readCurrentBoardId(options.getCurrentBoardId)
      const taskSubmission = createTaskSubmissionCallbacks({
        readinessRequest: () => buildImageActionReadinessRequest(plan.action),
        buildRequest: async () =>
          buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
      })
      await runPendingImageGeneration({
        source: plan.source,
        sourceNodeId: plan.sourceNodeId,
        resultOptions: plan.resultOptions,
        boardId,
        assertReady: taskSubmission.assertReady,
        submitTask: taskSubmission.submitTask,
        resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.source.ratio),
        successMessage: '处理完成',
        fallbackErrorMessage: '执行失败',
      })
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    try {
      const plan = planner.planCanvasExtendAction(nodeId, intent)
      const boardId = readCurrentBoardId(options.getCurrentBoardId)
      const taskSubmission = createTaskSubmissionCallbacks({
        readinessRequest: buildCanvasExtendReadinessRequest,
        buildRequest: async () =>
          buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
      })
      await runPendingImageBatchWorkflow({
        source: plan.source,
        sourceNodeId: plan.sourceNodeId,
        boardId,
        outputCount: plan.outputCount,
        resultOptions: plan.resultOptions,
        assertReady: taskSubmission.assertReady,
        submitTask: taskSubmission.submitTask,
        resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.targetRatio),
        fallbackErrorMessage: '扩图失败',
      })
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImageInpaintAction(nodeId: string, inpaint: RunImageInpaintActionOptions) {
    try {
      const plan = planner.planImageInpaintAction(nodeId, inpaint)
      let localInpaint: PreparedFlowLocalInpaintMedia | null = null
      let request: SubmitTaskData | null = null
      const boardId = readCurrentBoardId(options.getCurrentBoardId)
      const taskSubmission = createTaskSubmissionCallbacks({
        buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
        fallbackMessage: '提交局部重绘任务失败',
      })
      await runPendingImageBatchWorkflow({
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
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio = 'auto',
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    try {
      const plan = planner.planImagePromptGeneration(
        nodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      )
      await runImagePromptGenerationPlan(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGenerationFromSource(
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor } = {},
  ) {
    try {
      const plan = planner.planImagePromptGenerationFromSource(
        source,
        sourceNodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      )
      await runImagePromptGenerationPlan(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGenerationPlan(
    plan: ReturnType<typeof planner.planImagePromptGenerationFromSource>,
  ) {
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
    })
    await runPendingImageBatchWorkflow({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.source.ratio),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    try {
      const plan = planner.planTextPromptGeneration(anchorNodeId, prompt, outputCount, aspectRatio)
      const boardId = readCurrentBoardId(options.getCurrentBoardId)
      const taskSubmission = createTaskSubmissionCallbacks({
        buildRequest: () => buildImageExecutionTaskRequest(plan),
      })
      await runPendingImageBatchWorkflow({
        source: plan.source,
        sourceNodeId: plan.sourceNodeId,
        boardId,
        outputCount: plan.outputCount,
        resultOptions: plan.resultOptions,
        submitTask: taskSubmission.submitTask,
        resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, 1),
        fallbackErrorMessage: '生成失败',
      })
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    try {
      const plan = planner.planFusionPromptGeneration(nodeId, prompt, outputCount, aspectRatio)
      const boardId = readCurrentBoardId(options.getCurrentBoardId)
      let targetRatio = plan.source.ratio
      let request: SubmitTaskData | null = null
      const taskSubmission = createTaskSubmissionCallbacks({
        readinessRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
        buildRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
        fallbackMessage: '提交 Fusion 任务失败',
      })
      await runPendingImageBatchWorkflow({
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
        resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, targetRatio),
        fallbackErrorMessage: 'Fusion 生成失败',
        formatErrorMessage: (error) => sanitizeApiErrorMessage(error.message, 'Fusion 生成失败'),
      })
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  return {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    runImageDirectAction,
    runCanvasExtendAction,
    runImagePromptGeneration,
    runImagePromptGenerationFromSource,
    runImageInpaintAction,
    runFusionPromptGeneration,
    runTextPromptGeneration,
  }
}
