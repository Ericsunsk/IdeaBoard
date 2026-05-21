import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  buildExecutionTaskRequest,
  EXECUTION_EDIT_MODE_META,
  EXECUTION_FACE_SWAP_META,
  EXECUTION_MODEL_META,
  EXECUTION_VIEWPOINT_META,
  EXECUTION_WEAR_META,
  type ExecutionFeature,
} from '../config/executionContract'
import { assertTaskSubmitReady } from '../services/flowTaskExecutorUtils'
import { executeFlowEditNode, validateFlowEditNodeInputs } from '../services/flowEditExecutor'
import { executeFlowWearNode, validateFlowWearNodeInputs } from '../services/flowWearExecutor'
import { executeFlowModelNode, validateFlowModelNodeInputs } from '../services/flowModelExecutor'
import {
  executeFlowViewpointNode,
  validateFlowViewpointNodeInputs,
} from '../services/flowViewpointExecutor'
import type {
  CanvasExecutionOutputNodeType,
  EditNodeItem,
  ModelNodeItem,
  ViewpointNodeItem,
  WearNodeItem,
} from '../types/canvasNodes'
import type { SubmitTaskData } from '../../../shared/api/task'
import type { FlowHistorySnapshot } from './useFlowTransactions'

type FlowRuntimeNode = { id: string; data?: { kind?: string; snapshotNode?: unknown } }
type FlowRuntimeEdge = {
  source: string
  sourceHandle?: string | null
  target: string
  targetHandle?: string | null
}
type SnapshotNodeRecord = Record<string, unknown>
type UpdateSnapshotNodeOptions = {
  history?: boolean
  dirty?: boolean
  historyBefore?: FlowHistorySnapshot
}

type UseFlowGenerationHandlersOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  currentBoardId: Ref<string | null | undefined>
  updateSnapshotNode: (
    nodeId: string,
    patcher: (snapshotNode: SnapshotNodeRecord) => SnapshotNodeRecord,
    options?: UpdateSnapshotNodeOptions,
  ) => void
  captureHistory: () => FlowHistorySnapshot
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  /** Called after a successful generation commit; used to mark downstream nodes as stale. */
  onOutputCommitted?: (nodeId: string) => void
}

export function loadImageSizeFromUrl(
  url: string,
): Promise<{ width: number; height: number } | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (!img.naturalWidth || !img.naturalHeight) {
        resolve(null)
        return
      }
      resolve({ width: img.naturalWidth, height: img.naturalHeight })
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

export function useFlowGenerationHandlers(options: UseFlowGenerationHandlersOptions) {
  const runningEditNodeIds = ref(new Set<string>())
  const runningWearNodeIds = ref(new Set<string>())
  const runningModelNodeIds = ref(new Set<string>())
  const runningViewpointNodeIds = ref(new Set<string>())

  function isFlowExecutionNodeRunning(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: string,
  ): boolean {
    switch (nodeType) {
      case 'edit':
        return runningEditNodeIds.value.has(nodeId)
      case 'fusion':
        return false
      case 'wear':
        return runningWearNodeIds.value.has(nodeId)
      case 'model':
        return runningModelNodeIds.value.has(nodeId)
      case 'viewpoint':
        return runningViewpointNodeIds.value.has(nodeId)
      default:
        return false
    }
  }

  type GenerationNodeConfig<TContext, TOptions> = {
    nodeType: string
    runningSet: { value: Set<string> }
    runningMessage: string
    missingNodeMessage: string
    successMessage: string
    failureMessage: string
    errorMessage: string
    buildContext: (target: { data: { snapshotNode: unknown } }, nodeId: string) => TContext
    buildOptions: () => TOptions
    validate: (context: TContext, opts: TOptions) => string | null
    preflight?: (context: TContext, opts: TOptions) => Promise<void>
    execute: (context: TContext, opts: TOptions) => Promise<{ urls: string[] }>
    preserveOutputImagesOnFailure?: boolean
  }

  function buildExecutionReadinessRequest(feature: ExecutionFeature): SubmitTaskData {
    return buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature,
    })
  }

  async function assertExecutionSubmitReady(feature: ExecutionFeature): Promise<void> {
    await assertTaskSubmitReady(buildExecutionReadinessRequest(feature), '提交任务失败')
  }

  async function runFlowGenerationNode<TContext, TOptions>(
    nodeId: string,
    config: GenerationNodeConfig<TContext, TOptions>,
  ) {
    if (config.runningSet.value.has(nodeId)) {
      options.onWarning(config.runningMessage)
      return
    }
    const target = options.nodes.value.find((item) => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning(config.missingNodeMessage)
      return
    }

    const snapshot = target.data.snapshotNode as Record<string, unknown>
    const prevOutputStatus = (snapshot.outputStatus as string | undefined) ?? 'idle'
    const prevOutputImages = Array.isArray(snapshot.outputImages)
      ? (snapshot.outputImages as Array<{ url: string }>)
      : []
    const context = config.buildContext(target as { data: { snapshotNode: unknown } }, nodeId)
    const opts = config.buildOptions()
    const validationError = config.validate(context, opts)
    if (validationError) {
      options.onWarning(validationError)
      return
    }
    if (config.preflight) {
      try {
        await config.preflight(context, opts)
      } catch (error) {
        const msg = error instanceof Error ? error.message : config.failureMessage
        options.onWarning(msg)
        return
      }
    }

    config.runningSet.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await config.execute(context, opts)
      const outputImages = buildOutputImagesWithRatio(nodeId, urls)
      commitNodeOutputWithHistory(
        nodeId,
        (prev) => ({
          ...prev,
          outputImages,
          outputStatus: 'ready',
        }),
        beforeGenerate,
      )
      options.onSuccess(config.successMessage)
    } catch (error) {
      const msg = error instanceof Error ? error.message : config.failureMessage
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, (prev) => ({
        ...prev,
        outputStatus:
          prevOutputStatus ??
          (Array.isArray(prev.outputImages) && prev.outputImages.length > 0 ? 'ready' : 'idle'),
        outputImages:
          config.preserveOutputImagesOnFailure && prevOutputImages.length > 0
            ? prevOutputImages
            : Array.isArray(prev.outputImages)
              ? prev.outputImages
              : [],
      }))
      options.onError(config.errorMessage)
    } finally {
      config.runningSet.value.delete(nodeId)
    }
  }

  function captureGenerateHistoryBefore(): FlowHistorySnapshot {
    return options.captureHistory()
  }

  function setNodeOutputStaleSilently(nodeId: string) {
    options.updateSnapshotNode(nodeId, (prev) => ({ ...prev, outputStatus: 'stale' }), {
      history: false,
    })
  }

  function buildOutputImagesWithRatio(
    nodeId: string,
    urls: string[],
  ): Array<{ url: string; ratio: number }> {
    const initial = urls.map((url) => ({ url, ratio: 1 }))
    Promise.all(
      urls.map(async (url) => {
        const size = await loadImageSizeFromUrl(url)
        if (size && size.width > 0 && size.height > 0) {
          return size.height / size.width
        }
        return 1
      }),
    )
      .then((ratios) => {
        const nextImages = urls.map((url, i) => ({ url, ratio: ratios[i] ?? 1 }))
        options.updateSnapshotNode(
          nodeId,
          (prev) => {
            const currentOutputImages = Array.isArray(prev.outputImages)
              ? (prev.outputImages as Array<{ url?: unknown }>)
              : []
            const currentUrls = currentOutputImages.map((item) => String(item?.url ?? ''))
            if (
              currentUrls.length !== urls.length ||
              currentUrls.some((url, index) => url !== urls[index])
            ) {
              return prev
            }
            return {
              ...prev,
              outputImages: nextImages,
            }
          },
          { history: false, dirty: true },
        )
      })
      .catch(() => {})
    return initial
  }

  function commitNodeOutputWithHistory(
    nodeId: string,
    patcher: (snapshotNode: SnapshotNodeRecord) => SnapshotNodeRecord,
    beforeGenerate: FlowHistorySnapshot,
  ) {
    options.updateSnapshotNode(nodeId, patcher, { history: true, historyBefore: beforeGenerate })
    options.onOutputCommitted?.(nodeId)
  }

  function rollbackNodeOutputSilently(
    nodeId: string,
    patcher: (snapshotNode: SnapshotNodeRecord) => SnapshotNodeRecord,
  ) {
    options.updateSnapshotNode(nodeId, patcher, { history: false })
  }

  async function onFlowEditGenerate(nodeId: string) {
    await runFlowGenerationNode(nodeId, {
      nodeType: 'edit',
      runningSet: runningEditNodeIds,
      runningMessage: 'Edit 任务正在执行中',
      missingNodeMessage: 'Edit 节点不存在',
      successMessage: 'Edit 生成完成',
      failureMessage: 'Edit 生成失败',
      errorMessage: 'Edit 生成失败',
      buildContext: (target, nodeId) => ({
        nodeId,
        snapshotNode: target.data.snapshotNode as EditNodeItem,
      }),
      buildOptions: () => ({
        nodes: options.nodes.value as FlowRuntimeNode[],
        edges: options.edges.value as FlowRuntimeEdge[],
        loadImageSizeFromUrl,
        boardId: options.currentBoardId.value || undefined,
      }),
      validate: (ctx, opts) => validateFlowEditNodeInputs(ctx, opts),
      preflight: (ctx) =>
        assertExecutionSubmitReady(
          ctx.snapshotNode.mode === 'face-swap'
            ? EXECUTION_FACE_SWAP_META.feature
            : EXECUTION_EDIT_MODE_META[ctx.snapshotNode.mode].feature,
        ),
      execute: (ctx, opts) => executeFlowEditNode(ctx, opts),
    })
  }

  async function onFlowWearGenerate(nodeId: string) {
    await runFlowGenerationNode(nodeId, {
      nodeType: 'wear',
      runningSet: runningWearNodeIds,
      runningMessage: 'Wear 任务正在执行中',
      missingNodeMessage: 'Wear 节点不存在',
      successMessage: 'Wear 生成完成',
      failureMessage: 'Wear 生成失败',
      errorMessage: 'Wear 生成失败',
      preserveOutputImagesOnFailure: true,
      buildContext: (target, nodeId) => ({
        nodeId,
        snapshotNode: target.data.snapshotNode as WearNodeItem,
      }),
      buildOptions: () => ({
        nodes: options.nodes.value as FlowRuntimeNode[],
        edges: options.edges.value as FlowRuntimeEdge[],
        loadImageSizeFromUrl,
        boardId: options.currentBoardId.value || undefined,
      }),
      validate: (ctx, opts) => validateFlowWearNodeInputs(ctx, opts),
      preflight: () => assertExecutionSubmitReady(EXECUTION_WEAR_META.feature),
      execute: (ctx, opts) => executeFlowWearNode(ctx, opts),
    })
  }

  async function onFlowModelGenerate(nodeId: string) {
    await runFlowGenerationNode(nodeId, {
      nodeType: 'model',
      runningSet: runningModelNodeIds,
      runningMessage: 'Model 任务正在执行中',
      missingNodeMessage: 'Model 节点不存在',
      successMessage: 'Model 生成完成',
      failureMessage: 'Model 生成失败',
      errorMessage: 'Model 生成失败',
      preserveOutputImagesOnFailure: true,
      buildContext: (target, nodeId) => ({
        nodeId,
        snapshotNode: target.data.snapshotNode as ModelNodeItem,
      }),
      buildOptions: () => ({
        nodes: options.nodes.value as FlowRuntimeNode[],
        edges: options.edges.value as FlowRuntimeEdge[],
        loadImageSizeFromUrl,
        boardId: options.currentBoardId.value || undefined,
      }),
      validate: (ctx, opts) => validateFlowModelNodeInputs(ctx, opts),
      preflight: () => assertExecutionSubmitReady(EXECUTION_MODEL_META.feature),
      execute: (ctx, opts) => executeFlowModelNode(ctx, opts),
    })
  }

  async function onFlowViewpointGenerate(nodeId: string) {
    await runFlowGenerationNode(nodeId, {
      nodeType: 'viewpoint',
      runningSet: runningViewpointNodeIds,
      runningMessage: 'Viewpoint 任务正在执行中',
      missingNodeMessage: 'Viewpoint 节点不存在',
      successMessage: 'Viewpoint 生成完成',
      failureMessage: 'Viewpoint 生成失败',
      errorMessage: 'Viewpoint 生成失败',
      preserveOutputImagesOnFailure: true,
      buildContext: (target, nodeId) => ({
        nodeId,
        snapshotNode: target.data.snapshotNode as ViewpointNodeItem,
      }),
      buildOptions: () => ({
        nodes: options.nodes.value as FlowRuntimeNode[],
        edges: options.edges.value as FlowRuntimeEdge[],
        loadImageSizeFromUrl,
        boardId: options.currentBoardId.value || undefined,
      }),
      validate: (ctx, opts) => validateFlowViewpointNodeInputs(ctx, opts),
      preflight: () => assertExecutionSubmitReady(EXECUTION_VIEWPOINT_META.feature),
      execute: (ctx, opts) => executeFlowViewpointNode(ctx, opts),
    })
  }

  return {
    onFlowEditGenerate,
    onFlowWearGenerate,
    onFlowModelGenerate,
    onFlowViewpointGenerate,
    isFlowExecutionNodeRunning,
  }
}
