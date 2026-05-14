import { nextTick, ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { taskApi } from '../../../shared/api/task'
import { vectorizeApi } from '../../../shared/api/vectorize'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  buildExecutionTaskRequest,
  EXECUTION_DIRECT_ACTION_META,
  EXECUTION_EDIT_MODE_META,
} from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { createImageNodeAt } from '../utils/canvasNodeFactory'
import type { ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { assertTaskSubmitReady, waitTaskResult } from '../services/flowTaskExecutorUtils'
import { resolveImageNodeFrameSize, resolveImageNodeStageHeight } from '../utils/imageNodeSizing'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'

type DirectActionKey = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'
type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }
type RunImageDirectActionOptions = {
  strength?: number
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  captureHistory: () => FlowHistorySnapshot
  pushHistory: (snapshot: FlowHistorySnapshot) => void
  markDirty: () => void
  refreshSnapshot: () => void
  refreshNodeInternals?: (nodeIds: string[]) => void
  commitNodes: (nextNodes: Node[]) => void
  commitEdges: (nextEdges: Edge[]) => void
  appendEdges?: (nextEdges: Edge[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
}

type ImageDirectActionSource = {
  url: string
  ratio: number
  x: number
  y: number
  width: number
}

const STYLE_INNOVATE_DEFAULT_STRENGTH = 50
const IMAGE_RESULT_NODE_X_GAP = 100
const IMAGE_RESULT_NODE_Y_GAP = 14
const IMAGE_NODE_HEADER_HEIGHT = 26

function estimateImageNodeHeight(source: Pick<ImageDirectActionSource, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const cancelledResultNodeIds = new Set<string>()
  const resultLayoutsBySourceNodeId = new Map<string, Map<string, ImageDirectActionSource>>()

  function setNodeRunning(nodeId: string, running: boolean) {
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
  }

  function waitForFlowRenderFrame(): Promise<void> {
    if (typeof window === 'undefined') return nextTick()
    return nextTick().then(
      () =>
        new Promise((resolve) => {
          window.requestAnimationFrame(() => resolve())
        }),
    )
  }

  function getDirectActionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || target.data?.kind !== 'image') return null
    return target as Node<FlowNodeData>
  }

  function readImageSnapshot(nodeId: string): ImageDirectActionSource | null {
    const node = getDirectActionNodeById(nodeId)
    if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    const snapshot = node.data.snapshotNode as ImageNodeItem
    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
    const url = `${latestOutput?.url || snapshot.sourceImageUrl || ''}`.trim()
    if (!url) return null
    const outputRatio = Number(latestOutput?.ratio)
    const sourceRatio = Number(snapshot.sourceImageRatio)
    const ratio =
      Number.isFinite(outputRatio) && outputRatio > 0
        ? outputRatio
        : Number.isFinite(sourceRatio) && sourceRatio > 0
          ? sourceRatio
          : 1
    const width = Number(snapshot.width)
    return {
      url,
      ratio,
      x: Number(node.position?.x ?? snapshot.x ?? 0),
      y: Number(node.position?.y ?? snapshot.y ?? 0),
      width: Number.isFinite(width) && width > 0 ? width : 280,
    }
  }

  function commitGraphMutation(
    mutator: (items: Node[], edges: Edge[]) => { nodes: Node[]; edges: Edge[] },
    optionsOverride?: { recordHistory?: boolean },
  ) {
    const historyBefore = options.captureHistory()
    const { nodes: nextNodes, edges: nextEdges } = mutator(
      options.nodes.value,
      options.edges.value,
    )
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    if (optionsOverride?.recordHistory !== false) {
      options.pushHistory(historyBefore)
    }
    options.markDirty()
    options.refreshSnapshot()
  }

  function commitNodeMutation(
    mutator: (items: Node[]) => Node[],
    optionsOverride?: { recordHistory?: boolean },
  ) {
    const historyBefore = options.captureHistory()
    options.commitNodes(mutator(options.nodes.value))
    if (optionsOverride?.recordHistory !== false) {
      options.pushHistory(historyBefore)
    }
    options.markDirty()
    options.refreshSnapshot()
  }

  function readImageNodeLayout(node: Node): ImageDirectActionSource | null {
    if (node.data?.kind !== 'image') return null
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    const imageNode = snapshot as ImageNodeItem
    const outputs = Array.isArray(imageNode.outputImages) ? imageNode.outputImages : []
    const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
    const width = Number(imageNode.width)
    const outputRatio = Number(latestOutput?.ratio)
    const sourceRatio = Number(imageNode.sourceImageRatio)
    const ratio =
      Number.isFinite(outputRatio) && outputRatio > 0
        ? outputRatio
        : Number.isFinite(sourceRatio) && sourceRatio > 0
          ? sourceRatio
          : 1
    return {
      url: `${latestOutput?.url || imageNode.sourceImageUrl || ''}`.trim(),
      ratio,
      x: Number(node.position?.x ?? imageNode.x ?? 0),
      y: Number(node.position?.y ?? imageNode.y ?? 0),
      width: Number.isFinite(width) && width > 0 ? width : 280,
    }
  }

  function resolvePendingImageResultPosition(source: ImageDirectActionSource, sourceNodeId: string) {
    const defaultPosition = {
      x: source.x + source.width + IMAGE_RESULT_NODE_X_GAP,
      y: source.y,
    }
    const childNodeIds = new Set<string>(
      options.edges.value
        .filter(
          (edge) =>
            edge.source === sourceNodeId &&
            edge.sourceHandle === 'out:image' &&
            edge.targetHandle === 'in:image',
        )
        .map((edge) => edge.target),
    )
    const runtimeLayouts = resultLayoutsBySourceNodeId.get(sourceNodeId)
    runtimeLayouts?.forEach((_, nodeId) => childNodeIds.add(nodeId))
    if (childNodeIds.size === 0) return defaultPosition

    let nextY = source.y
    let alignedX = defaultPosition.x
    const measuredNodeIds = new Set<string>()
    const visitChildLayout = (nodeId: string, layout: ImageDirectActionSource) => {
      const bottomY = layout.y + estimateImageNodeHeight(layout) + IMAGE_RESULT_NODE_Y_GAP
      if (bottomY >= nextY) {
        alignedX = layout.x
        nextY = bottomY
      }
      measuredNodeIds.add(nodeId)
    }

    for (const node of options.nodes.value) {
      if (!childNodeIds.has(node.id)) continue
      const layout = readImageNodeLayout(node) ?? runtimeLayouts?.get(node.id)
      if (!layout) continue
      visitChildLayout(node.id, layout)
    }
    runtimeLayouts?.forEach((layout, nodeId) => {
      if (!childNodeIds.has(nodeId)) return
      if (measuredNodeIds.has(nodeId)) return
      visitChildLayout(nodeId, layout)
    })

    return {
      x: alignedX,
      y: nextY,
    }
  }

  function rememberResultNodeLayout(
    sourceNodeId: string,
    resultNodeId: string,
    layout: ImageDirectActionSource,
  ) {
    const current = resultLayoutsBySourceNodeId.get(sourceNodeId) ?? new Map()
    current.set(resultNodeId, layout)
    resultLayoutsBySourceNodeId.set(sourceNodeId, current)
  }

  function forgetResultNodeLayout(resultNodeId: string) {
    resultLayoutsBySourceNodeId.forEach((items, sourceNodeId) => {
      items.delete(resultNodeId)
      if (items.size === 0) resultLayoutsBySourceNodeId.delete(sourceNodeId)
    })
  }

  function updateResultNodeLayout(resultNodeId: string, patch: Partial<ImageDirectActionSource>) {
    resultLayoutsBySourceNodeId.forEach((items) => {
      const current = items.get(resultNodeId)
      if (!current) return
      items.set(resultNodeId, { ...current, ...patch })
    })
  }

  function pruneResultNodeLayouts() {
    const existingNodeIds = new Set(options.nodes.value.map((node) => node.id))
    resultLayoutsBySourceNodeId.forEach((items, sourceNodeId) => {
      items.forEach((_, nodeId) => {
        if (!existingNodeIds.has(nodeId)) items.delete(nodeId)
      })
      if (items.size === 0) resultLayoutsBySourceNodeId.delete(sourceNodeId)
    })
  }

  async function appendPendingImageResultEdge(sourceNodeId: string, resultNodeId: string) {
    await waitForFlowRenderFrame()
    if (cancelledResultNodeIds.has(resultNodeId)) return
    const hasSource = options.nodes.value.some((node) => node.id === sourceNodeId)
    const targetNode = options.nodes.value.find((node) => node.id === resultNodeId)
    const hasTarget = !!targetNode
    if (!hasSource || !hasTarget) return
    const targetSnapshot =
      targetNode.data?.snapshotNode && typeof targetNode.data.snapshotNode === 'object'
        ? (targetNode.data.snapshotNode as ImageNodeItem)
        : null
    options.refreshNodeInternals?.([sourceNodeId, resultNodeId])
    const nextEdge: Edge = {
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: sourceNodeId,
      sourceHandle: 'out:image',
      target: resultNodeId,
      targetHandle: 'in:image',
      animated: targetSnapshot?.outputStatus === 'running',
    }
    const hasSameEdge = options.edges.value.some(
      (edge) =>
        edge.source === nextEdge.source &&
        edge.target === nextEdge.target &&
        edge.sourceHandle === nextEdge.sourceHandle &&
        edge.targetHandle === nextEdge.targetHandle,
    )
    if (hasSameEdge) return
    if (options.appendEdges) {
      options.appendEdges([nextEdge])
    } else {
      options.commitEdges([...options.edges.value, nextEdge])
    }
    options.markDirty()
    options.refreshSnapshot()
    options.refreshNodeInternals?.([sourceNodeId, resultNodeId])
  }

  function appendPendingImageResultNode(
    source: ImageDirectActionSource,
    sourceNodeId: string,
  ): string {
    pruneResultNodeLayouts()
    const position = resolvePendingImageResultPosition(source, sourceNodeId)
    const imageNode = createImageNodeAt(position)
    imageNode.sourceImageUrl = ''
    imageNode.sourceImageRatio = source.ratio > 0 ? source.ratio : 1
    imageNode.width = resolveImageNodeFrameSize(imageNode.sourceImageRatio, true).width
    imageNode.outputStatus = 'running'
    imageNode.outputImages = []

    const nextNode: Node<FlowNodeData> = {
      id: `image-${imageNode.id}`,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('image'),
      position: { x: imageNode.x, y: imageNode.y },
      data: {
        kind: 'image',
        nodeType: 'image',
        title: `Image #${imageNode.id}`,
        summary: 'Canvas image node',
        snapshotNode: imageNode,
      },
    }

    commitNodeMutation((items) => [...items, nextNode as Node])
    cancelledResultNodeIds.delete(nextNode.id)
    rememberResultNodeLayout(sourceNodeId, nextNode.id, {
      url: '',
      ratio: imageNode.sourceImageRatio,
      x: imageNode.x,
      y: imageNode.y,
      width: imageNode.width,
    })
    options.refreshNodeInternals?.([sourceNodeId, nextNode.id])
    void appendPendingImageResultEdge(sourceNodeId, nextNode.id)

    return nextNode.id
  }

  function updatePendingImageResultNode(nodeId: string, url: string, ratio: number) {
    const frameSize = resolveImageNodeFrameSize(ratio, true)
    commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.map((item) => {
          if (item.id !== nodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          return {
            ...item,
            data: {
              ...item.data,
              snapshotNode: {
                ...(snapshot as ImageNodeItem),
                width: frameSize.width,
                sourceImageUrl: url,
                sourceImageRatio: ratio > 0 ? ratio : 1,
                outputStatus: 'ready',
                outputImages: [],
              },
            },
          }
        }),
        edges: currentEdges.map((edge) =>
          edge.target === nodeId && edge.targetHandle === 'in:image'
            ? { ...edge, animated: false }
            : edge,
        ),
      }),
      { recordHistory: false },
    )
    updateResultNodeLayout(nodeId, { url, ratio: ratio > 0 ? ratio : 1 })
    options.refreshNodeInternals?.([nodeId])
  }

  function removePendingImageResultNode(nodeId: string) {
    cancelledResultNodeIds.add(nodeId)
    forgetResultNodeLayout(nodeId)
    commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.filter((item) => item.id !== nodeId),
        edges: currentEdges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
      }),
      { recordHistory: false },
    )
  }

  async function resolveResultRatio(url: string, fallbackRatio: number): Promise<number> {
    try {
      const size = await loadImageSizeFromUrl(url)
      if (size && size.width > 0 && size.height > 0) {
        return size.height / size.width
      }
    } catch {
      // Non-critical: layout can use the source ratio if result probing fails.
    }
    return fallbackRatio > 0 ? fallbackRatio : 1
  }

  async function assertDirectActionSubmitReady(
    action: Exclude<DirectActionKey, 'to-vector'>,
  ): Promise<void> {
    const feature =
      action === 'style-innovate'
        ? EXECUTION_DIRECT_ACTION_META.styleInnovate.feature
        : action === 'remove-bg'
          ? EXECUTION_DIRECT_ACTION_META.removeBg.feature
          : EXECUTION_DIRECT_ACTION_META.sketchToDesign.feature
    await assertTaskSubmitReady(
      buildExecutionTaskRequest({
        prompt: 'submit readiness',
        feature,
      }),
      '提交任务失败',
    )
  }

  async function assertPromptImageSubmitReady(): Promise<void> {
    await assertTaskSubmitReady(
      buildExecutionTaskRequest({
        prompt: 'submit readiness',
        feature: EXECUTION_EDIT_MODE_META['partial-edit'].feature,
      }),
      '提交任务失败',
    )
  }

  async function runSubmitAndWait(
    source: ImageDirectActionSource,
    action: Exclude<DirectActionKey, 'to-vector'>,
    strengthOverride?: number,
  ): Promise<string[]> {
    const imageSize = await loadImageSizeFromUrl(source.url)
    const strength = Number.isFinite(strengthOverride)
      ? Math.max(0, Math.min(100, Math.round(strengthOverride as number)))
      : STYLE_INNOVATE_DEFAULT_STRENGTH
    const request =
      action === 'style-innovate'
        ? buildExecutionTaskRequest({
            prompt: '',
            feature: EXECUTION_DIRECT_ACTION_META.styleInnovate.feature,
            imageUrls: [source.url],
            params: {
              original_image_url: source.url,
              ...buildExecutionSizeParams(imageSize),
              variation_strength: strength,
              strength,
            },
          })
        : buildExecutionTaskRequest({
            prompt: '',
            feature:
              action === 'remove-bg'
                ? EXECUTION_DIRECT_ACTION_META.removeBg.feature
                : EXECUTION_DIRECT_ACTION_META.sketchToDesign.feature,
            imageUrls: [source.url],
            params: buildExecutionSizeParams(imageSize),
          })

    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
    }
    return waitTaskResult(submitRes.data.task_id)
  }

  async function runPromptImageGeneration(
    source: ImageDirectActionSource,
    prompt: string,
  ): Promise<string[]> {
    const imageSize = await loadImageSizeFromUrl(source.url)
    const request = buildExecutionTaskRequest({
      prompt: `${EXECUTION_EDIT_MODE_META['partial-edit'].promptPrefix} ${prompt}`,
      feature: EXECUTION_EDIT_MODE_META['partial-edit'].feature,
      imageUrls: [source.url],
      params: {
        original_image_url: source.url,
        ...buildExecutionSizeParams(imageSize),
      },
    })

    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
    }
    return waitTaskResult(submitRes.data.task_id)
  }

  async function runToVector(source: ImageDirectActionSource): Promise<string[]> {
    const res = await vectorizeApi.vectorize({
      image_url: source.url,
      threshold: 0.45,
      turd_size: 2,
      alpha_max: 1.0,
      opt_tolerance: 0.2,
      turn_policy: 'minority',
      invert: false,
      color_count: 0,
    })
    if (res.code !== 0 || !res.data?.svg_url) {
      throw new Error(sanitizeApiErrorMessage(res.msg, 'Vectorize failed'))
    }
    return [res.data.svg_url]
  }

  async function runImageDirectAction(
    nodeId: string,
    action: DirectActionKey,
    runOptions?: RunImageDirectActionOptions,
  ) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    let resultNodeId: string | null = null
    try {
      if (action !== 'to-vector') {
        await assertDirectActionSubmitReady(action)
      }
      resultNodeId = appendPendingImageResultNode(source, nodeId)
      setNodeRunning(resultNodeId, true)
      const urls =
        action === 'to-vector'
          ? await runToVector(source)
          : await runSubmitAndWait(source, action, runOptions?.strength)
      const firstUrl = urls.find((url) => typeof url === 'string' && url.trim())?.trim()
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      const ratio = await resolveResultRatio(firstUrl, source.ratio)
      updatePendingImageResultNode(resultNodeId, firstUrl, ratio)
      options.onSuccess('处理完成')
    } catch (error) {
      if (resultNodeId) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '执行失败'
      options.onWarning(msg)
    } finally {
      if (resultNodeId) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runImagePromptGeneration(nodeId: string, prompt: string) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    let resultNodeId: string | null = null
    try {
      await assertPromptImageSubmitReady()
      resultNodeId = appendPendingImageResultNode(source, nodeId)
      setNodeRunning(resultNodeId, true)
      const urls = await runPromptImageGeneration(source, normalizedPrompt)
      const firstUrl = urls.find((url) => typeof url === 'string' && url.trim())?.trim()
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      const ratio = await resolveResultRatio(firstUrl, source.ratio)
      updatePendingImageResultNode(resultNodeId, firstUrl, ratio)
      options.onSuccess('生成完成')
    } catch (error) {
      if (resultNodeId) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '生成失败'
      options.onWarning(msg)
    } finally {
      if (resultNodeId) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    runImageDirectAction,
    runImagePromptGeneration,
  }
}
