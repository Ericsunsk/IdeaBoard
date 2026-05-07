import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { executeFlowEnhanceNode, validateFlowEnhanceNodeInputs } from '../services/flowEnhanceExecutor'
import { executeFlowColorNode, validateFlowColorNodeInputs } from '../services/flowColorExecutor'
import { executeFlowEditNode, validateFlowEditNodeInputs } from '../services/flowEditExecutor'
import { executeFlowFusionNode, validateFlowFusionNodeInputs } from '../services/flowFusionExecutor'
import { executeFlowWearNode, validateFlowWearNodeInputs } from '../services/flowWearExecutor'
import { executeFlowModelNode, validateFlowModelNodeInputs } from '../services/flowModelExecutor'
import { executeFlowViewpointNode, validateFlowViewpointNodeInputs } from '../services/flowViewpointExecutor'
import type {
  CanvasExecutionOutputNodeType,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ModelNodeItem,
  ViewpointNodeItem,
  WearNodeItem,
} from '../types/canvasNodes'

type FlowRuntimeNode = { id: string; data?: { kind?: string; snapshotNode?: unknown } }
type FlowRuntimeEdge = { source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }
type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }
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

export function loadImageSizeFromUrl(url: string): Promise<{ width: number; height: number } | null> {
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
  const runningEnhanceNodeIds = ref(new Set<string>())
  const runningColorNodeIds = ref(new Set<string>())
  const runningEditNodeIds = ref(new Set<string>())
  const runningFusionNodeIds = ref(new Set<string>())
  const runningWearNodeIds = ref(new Set<string>())
  const runningModelNodeIds = ref(new Set<string>())
  const runningViewpointNodeIds = ref(new Set<string>())

  function isFlowExecutionNodeRunning(nodeType: CanvasExecutionOutputNodeType, nodeId: string): boolean {
    switch (nodeType) {
      case 'enhance':
        return runningEnhanceNodeIds.value.has(nodeId)
      case 'color':
        return runningColorNodeIds.value.has(nodeId)
      case 'edit':
        return runningEditNodeIds.value.has(nodeId)
      case 'fusion':
        return runningFusionNodeIds.value.has(nodeId)
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

  function captureGenerateHistoryBefore(): FlowHistorySnapshot {
    return options.captureHistory()
  }

  function setNodeOutputStaleSilently(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({ ...prev, outputStatus: 'stale' }), { history: false })
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

  async function onFlowEnhanceGenerate(nodeId: string) {
    if (runningEnhanceNodeIds.value.has(nodeId)) {
      options.onWarning('Enhance 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Enhance 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as EnhanceNodeItem).outputStatus
    const enhanceContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as EnhanceNodeItem,
    }
    const enhanceOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
      boardId: options.currentBoardId.value || undefined,
    }
    const validationError = validateFlowEnhanceNodeInputs(enhanceContext, enhanceOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningEnhanceNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowEnhanceNode(enhanceContext, enhanceOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Enhance 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Enhance 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputStatus: prevOutputStatus ?? (Array.isArray(prev.outputImages) && prev.outputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Enhance 生成失败')
    } finally {
      runningEnhanceNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowEditGenerate(nodeId: string) {
    if (runningEditNodeIds.value.has(nodeId)) {
      options.onWarning('Edit 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Edit 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as EditNodeItem).outputStatus
    const editContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as EditNodeItem,
    }
    const editOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
      boardId: options.currentBoardId.value || undefined,
    }
    const validationError = validateFlowEditNodeInputs(editContext, editOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningEditNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowEditNode(editContext, editOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Edit 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Edit 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputStatus: prevOutputStatus ?? (Array.isArray(prev.outputImages) && prev.outputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Edit 生成失败')
    } finally {
      runningEditNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowColorGenerate(nodeId: string) {
    if (runningColorNodeIds.value.has(nodeId)) {
      options.onWarning('Color 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Color 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as ColorNodeItem).outputStatus
    const colorContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as ColorNodeItem,
    }
    const colorOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
      boardId: options.currentBoardId.value || undefined,
    }
    const validationError = validateFlowColorNodeInputs(colorContext, colorOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningColorNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowColorNode(colorContext, colorOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Color 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Color 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputStatus: prevOutputStatus ?? (Array.isArray(prev.outputImages) && prev.outputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Color 生成失败')
    } finally {
      runningColorNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowFusionGenerate(nodeId: string) {
    if (runningFusionNodeIds.value.has(nodeId)) {
      options.onWarning('Fusion 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Fusion 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as FusionNodeItem).outputStatus
    const fusionContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as FusionNodeItem,
    }
    const fusionOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
    }
    const validationError = validateFlowFusionNodeInputs(fusionContext, fusionOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningFusionNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowFusionNode(fusionContext, fusionOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Fusion 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Fusion 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputStatus: prevOutputStatus ?? (Array.isArray(prev.outputImages) && prev.outputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Fusion 生成失败')
    } finally {
      runningFusionNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowWearGenerate(nodeId: string) {
    if (runningWearNodeIds.value.has(nodeId)) {
      options.onWarning('Wear 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Wear 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as WearNodeItem).outputStatus
    const prevOutputImages = Array.isArray((target.data.snapshotNode as WearNodeItem).outputImages)
      ? [...(target.data.snapshotNode as WearNodeItem).outputImages]
      : []
    const wearContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as WearNodeItem,
    }
    const wearOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
      boardId: options.currentBoardId.value || undefined,
    }
    const validationError = validateFlowWearNodeInputs(wearContext, wearOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningWearNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowWearNode(wearContext, wearOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Wear 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Wear 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputImages: prevOutputImages,
        outputStatus: prevOutputStatus ?? (prevOutputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Wear 生成失败')
    } finally {
      runningWearNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowModelGenerate(nodeId: string) {
    if (runningModelNodeIds.value.has(nodeId)) {
      options.onWarning('Model 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Model 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as ModelNodeItem).outputStatus
    const prevOutputImages = Array.isArray((target.data.snapshotNode as ModelNodeItem).outputImages)
      ? [...(target.data.snapshotNode as ModelNodeItem).outputImages]
      : []
    const modelContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as ModelNodeItem,
    }
    const modelOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
    }
    const validationError = validateFlowModelNodeInputs(modelContext, modelOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningModelNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowModelNode(modelContext, modelOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Model 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Model 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputImages: prevOutputImages,
        outputStatus: prevOutputStatus ?? (prevOutputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Model 生成失败')
    } finally {
      runningModelNodeIds.value.delete(nodeId)
    }
  }

  async function onFlowViewpointGenerate(nodeId: string) {
    if (runningViewpointNodeIds.value.has(nodeId)) {
      options.onWarning('Viewpoint 任务正在执行中')
      return
    }
    const target = options.nodes.value.find(item => item.id === nodeId)
    if (!target?.data?.snapshotNode || typeof target.data.snapshotNode !== 'object') {
      options.onWarning('Viewpoint 节点不存在')
      return
    }

    const prevOutputStatus = (target.data.snapshotNode as ViewpointNodeItem).outputStatus
    const prevOutputImages = Array.isArray((target.data.snapshotNode as ViewpointNodeItem).outputImages)
      ? [...(target.data.snapshotNode as ViewpointNodeItem).outputImages]
      : []
    const viewpointContext = {
      nodeId,
      snapshotNode: target.data.snapshotNode as ViewpointNodeItem,
    }
    const viewpointOptions = {
      nodes: options.nodes.value as FlowRuntimeNode[],
      edges: options.edges.value as FlowRuntimeEdge[],
      loadImageSizeFromUrl,
    }
    const validationError = validateFlowViewpointNodeInputs(viewpointContext, viewpointOptions)
    if (validationError) {
      options.onWarning(validationError)
      return
    }

    runningViewpointNodeIds.value.add(nodeId)
    const beforeGenerate = captureGenerateHistoryBefore()
    try {
      setNodeOutputStaleSilently(nodeId)
      const { urls } = await executeFlowViewpointNode(viewpointContext, viewpointOptions)
      commitNodeOutputWithHistory(nodeId, prev => ({
        ...prev,
        outputImages: urls.map(url => ({ url, ratio: 1 })),
        outputStatus: 'ready',
      }), beforeGenerate)
      options.onSuccess('Viewpoint 生成完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Viewpoint 生成失败'
      options.onWarning(msg)
      rollbackNodeOutputSilently(nodeId, prev => ({
        ...prev,
        outputImages: prevOutputImages,
        outputStatus: prevOutputStatus ?? (prevOutputImages.length > 0 ? 'ready' : 'idle'),
      }))
      options.onError('Viewpoint 生成失败')
    } finally {
      runningViewpointNodeIds.value.delete(nodeId)
    }
  }

  return {
    onFlowEnhanceGenerate,
    onFlowEditGenerate,
    onFlowColorGenerate,
    onFlowFusionGenerate,
    onFlowWearGenerate,
    onFlowModelGenerate,
    onFlowViewpointGenerate,
    isFlowExecutionNodeRunning,
  }
}
