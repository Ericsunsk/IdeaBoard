import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphStore } from '../../graph-runtime'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { FlowToCanvasBoardSnapshotOptions } from '../../utils/flowSnapshotAdapter'
import { readCurrentBoardId } from '../../utils/currentBoardId'
import {
  buildCanvasExtendReadinessRequest,
  buildImageActionReadinessRequest,
  buildImageExecutionTaskRequest,
  buildPreparedImageInpaintTaskRequest,
  buildPromptImageReadinessRequest,
} from '../commands/flowImageTaskRequests'
import {
  prepareFlowLocalInpaintMedia,
  type PreparedFlowLocalInpaintMedia,
} from '../media/flowLocalInpaint'
import { loadImageSizeFromUrl } from '../media/imageSize'
import {
  prepareFusionExecutionPlan,
  type ActivatedImageExecutionPlan,
  type CanvasExtendExecutionPlan,
  type FusionExecutionPlan,
  type ImageActionExecutionPlan,
  type ImageInpaintExecutionPlan,
  type ImagePromptExecutionPlan,
  type TextPromptExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { resolveFirstTaskImageResultDisplay } from '../result-nodes/flowImageDirectActionTaskResult'
import { composeFlowLocalInpaintTaskResult } from '../result-nodes/flowLocalInpaintTaskResult'
import { usePendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import { createCanvasTaskSubmissionCallbacks } from '../workspace/canvasTaskSubmissionAdapter'
import {
  type CanvasExecutionWorkspace,
  useCanvasExecutionWorkspace,
} from '../workspace/useCanvasExecutionWorkspace'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'
import {
  projectCanvasExecutionEdgeAnimation,
  type CanvasExecutionStateChange,
} from './CanvasExecutionRuntime'
import { createFlowGraphCommandOptions } from '../../composables/flowGraphAdapterCommandSideEffects'

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
  getCurrentBoardId?: () => string
  executionWorkspace?: CanvasExecutionWorkspace
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
  })

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

  const executionWorkspace =
    options.executionWorkspace ??
    useCanvasExecutionWorkspace({
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
  const executionRuntime = executionWorkspace.state.executionRuntime

  function syncExecutionEdgeProjection(change: CanvasExecutionStateChange) {
    const projection = projectCanvasExecutionEdgeAnimation(
      options.edges.value,
      executionRuntime.nodeStates.value,
      [change.nodeId],
    )
    if (!projection.didChange) return
    options.graphAdapter.replaceEdges(
      projection.edges,
      createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
    )
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    isBoardCurrent: isBoardStillCurrent,
    executionRuntime,
    onExecutionStateChange: syncExecutionEdgeProjection,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return executionWorkspace.state.isNodeRunning(nodeId)
  }

  function readBoardId(): string {
    return readCurrentBoardId(options.getCurrentBoardId)
  }

  async function executeImageAction(
    plan: ImageActionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => buildImageActionReadinessRequest(plan.action),
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
      outputCount: () => 1,
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      resultOptions: plan.resultOptions,
      boardId,
      outputCount: 1,
      activationId: plan.activationId,
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.source.ratio),
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
  }

  async function executeCanvasExtend(
    plan: CanvasExtendExecutionPlan & { activationId: string },
  ): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildCanvasExtendReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
      outputCount: () => plan.outputCount,
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      activationId: plan.activationId,
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.targetRatio),
      fallbackErrorMessage: '扩图失败',
    })
  }

  async function executeImageInpaint(
    plan: ImageInpaintExecutionPlan & { activationId: string },
  ): Promise<void> {
    let localInpaint: PreparedFlowLocalInpaintMedia | null = null
    let request: SubmitTaskData | null = null
    const boardId = readBoardId()
    const readinessSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: buildPromptImageReadinessRequest,
      outputCount: () => plan.outputCount,
    })
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
      fallbackMessage: '提交局部重绘任务失败',
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      activationId: plan.activationId,
      preflight: readinessSubmission.assertReady,
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
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: async (taskId) => {
        if (!localInpaint) throw new Error('局部重绘任务未准备完成')
        async function waitLocalInpaintTaskResults() {
          while (true) {
            try {
              return await waitExistingOrLiveTaskImageResults(taskId)
            } catch (error) {
              if (!isTaskResultPendingError(error) || !isBoardStillCurrent(boardId)) {
                throw error
              }
            }
          }
        }
        return await composeFlowLocalInpaintTaskResult({
          originalImageUrl: plan.source.url,
          croppedTaskResults: await waitLocalInpaintTaskResults(),
          prepared: localInpaint,
          boardId,
          fallbackRatio: plan.source.ratio,
          loadImageSize: loadImageSizeFromUrl,
        })
      },
      persistPendingTaskIds: false,
      fallbackErrorMessage: '局部重绘失败',
      formatErrorMessage: (error) => {
        const message = error.message
        return isTaskResultPendingError(error) ? '局部重绘仍在处理中，请稍后重试' : message
      },
    })
  }

  async function executeImagePrompt(
    plan: ImagePromptExecutionPlan & { activationId: string },
  ): Promise<void> {
    const boardId = readBoardId()
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: async () =>
        buildImageExecutionTaskRequest(plan, await loadImageSizeFromUrl(plan.source.url)),
      outputCount: () => plan.outputCount,
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      activationId: plan.activationId,
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, plan.source.ratio),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function executeTextPrompt(
    plan: TextPromptExecutionPlan & { activationId: string },
  ): Promise<void> {
    await executionWorkspace.commands.runTextPromptGeneration(plan)
  }

  async function executeFusion(
    plan: FusionExecutionPlan & { activationId: string },
  ): Promise<void> {
    const boardId = readBoardId()
    let targetRatio = plan.source.ratio
    let request: SubmitTaskData | null = null
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      buildRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      fallbackMessage: '提交 Fusion 任务失败',
      outputCount: () => plan.outputCount,
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      boardId,
      outputCount: plan.outputCount,
      activationId: plan.activationId,
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
    getNodeExecutionState: executionRuntime.getNodeState,
    reattachPendingImageResults: executionWorkspace.commands.reattachPendingImageResults,
  }
}
