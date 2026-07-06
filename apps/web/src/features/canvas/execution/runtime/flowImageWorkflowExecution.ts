import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphNodeId, CanvasGraphStore } from '../../graph-runtime'
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
  type ImageExecutionPlan,
  type ImageInpaintExecutionPlan,
  type ImagePromptExecutionPlan,
  type TextPromptExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { resolveFirstTaskImageResultDisplay } from '../result-nodes/flowImageDirectActionTaskResult'
import {
  collectPendingImageTasks,
  isPendingImageTaskSnapshot,
  readPendingImageSnapshot,
} from '../result-nodes/flowPendingImageTaskSnapshot'
import { composeFlowLocalInpaintTaskResult } from '../result-nodes/flowLocalInpaintTaskResult'
import type { ResultMaterializerImageResult } from '../result-nodes/resultMaterializer'
import { usePendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import {
  assertTaskRequestReady,
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from '../task-runner/flowImageDirectActionExecutor'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'

type TaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
}

export type FlowImageWorkflowExecutionPlan = ImageExecutionPlan

export type UseFlowImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: () => CanvasGraphStore
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

export function useFlowImageWorkflowExecution(options: UseFlowImageWorkflowExecutionOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const activeReattachKeys = new Set<string>()
  const pendingResults = usePendingImageResultExecution({
    nodes: options.nodes,
    edges: options.edges,
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    refreshNodeInternals: options.refreshNodeInternals,
  })

  function setNodeRunning(nodeId: string, running: boolean) {
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
  }

  function isBoardStillCurrent(boardId: string): boolean {
    return !boardId || readCurrentBoardId(options.getCurrentBoardId) === boardId
  }

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning('画布保存失败，刷新页面后任务进度可能丢失')
    }
  }

  async function resolveFirstTaskImageResult(taskId: string, fallbackRatio: number) {
    return await resolveFirstTaskImageResultDisplay(
      await waitExistingOrLiveTaskImageResults(taskId),
      fallbackRatio,
      loadImageSizeFromUrl,
    )
  }

  function isPendingImageTaskStillCurrent(nodeId: string, taskId: string): boolean {
    const snapshot = readPendingImageSnapshot(options.nodes.value, nodeId)
    return !!snapshot && isPendingImageTaskSnapshot(snapshot, taskId)
  }

  function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
    return /^(?:image|prompt|fusion)-\d+(?:\.\d+)?$/.test(nodeId)
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    isBoardCurrent: isBoardStillCurrent,
    setNodeRunning,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })

  async function reattachPendingImageResults(): Promise<void> {
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const pendingTasks = collectPendingImageTasks(options.nodes.value)
    if (pendingTasks.length === 0) return
    let failureCount = 0
    const resolvedResults: ResultMaterializerImageResult[] = []
    await Promise.allSettled(
      pendingTasks.map(async ({ nodeId, taskId, fallbackRatio }) => {
        const reattachKey = `${boardId}:${nodeId}:${taskId}`
        if (activeReattachKeys.has(reattachKey) || runningNodeIds.value.has(nodeId)) return
        activeReattachKeys.add(reattachKey)
        setNodeRunning(nodeId, true)
        try {
          const resolved = await resolveFirstTaskImageResult(taskId, fallbackRatio)
          if (!isBoardStillCurrent(boardId) || !isPendingImageTaskStillCurrent(nodeId, taskId)) {
            return
          }
          if (!isCanvasGraphNodeId(nodeId)) return
          resolvedResults.push({
            ...resolved,
            resultNodeId: nodeId,
            taskId,
          })
        } catch (error) {
          if (!isBoardStillCurrent(boardId) || !isPendingImageTaskStillCurrent(nodeId, taskId)) {
            return
          }
          console.warn('[canvas] reattach pending image task failed', error)
          failureCount += 1
        } finally {
          activeReattachKeys.delete(reattachKey)
          setNodeRunning(nodeId, false)
        }
      }),
    )
    if (!isBoardStillCurrent(boardId)) return
    const materialized =
      resolvedResults.length > 0
        ? pendingResults.materializeResolvedAttachedPendingResults({
            results: resolvedResults,
            isAttached: (result) =>
              !!result.taskId && isPendingImageTaskStillCurrent(result.resultNodeId, result.taskId),
          })
        : null
    const successCount = materialized?.materializedNodeIds.length ?? 0
    if (successCount > 0) {
      options.onSuccess(successCount > 1 ? `已恢复 ${successCount} 个生成结果` : '已恢复生成结果')
      await flushPendingTaskSnapshot()
    }
    if (failureCount > 0) {
      options.onWarning(
        failureCount > 1 ? `${failureCount} 个生成任务恢复失败` : '生成任务恢复失败',
      )
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

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
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      boardId,
      outputCount: 1,
      assertReady: taskSubmission.assertReady,
      submitTask: () => taskSubmission.submitTask(),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.source.ratio),
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
    await workflowRunner.run({
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
  }

  async function executeImageInpaint(plan: ImageInpaintExecutionPlan): Promise<void> {
    let localInpaint: PreparedFlowLocalInpaintMedia | null = null
    let request: SubmitTaskData | null = null
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
      fallbackMessage: '提交局部重绘任务失败',
    })
    await workflowRunner.run({
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
    await workflowRunner.run({
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

  async function executeTextPrompt(plan: TextPromptExecutionPlan): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => buildImageExecutionTaskRequest(plan),
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      submitTask: taskSubmission.submitTask,
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, 1),
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
    await workflowRunner.run({
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
    isNodeDirectActionRunning,
    reattachPendingImageResults,
  }
}
