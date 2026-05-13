import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { taskApi } from '../../../shared/api/task'
import { vectorizeApi } from '../../../shared/api/vectorize'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  buildExecutionTaskRequest,
  EXECUTION_DIRECT_ACTION_META,
} from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { createImageNodeAt } from '../utils/canvasNodeFactory'
import type { ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { waitTaskResult } from '../services/flowTaskExecutorUtils'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'

type DirectActionKey = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'
type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }
type RunImageDirectActionOptions = {
  strength?: number
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  captureHistory: () => FlowHistorySnapshot
  pushHistory: (snapshot: FlowHistorySnapshot) => void
  markDirty: () => void
  refreshSnapshot: () => void
  commitNodes: (nextNodes: Node[]) => void
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

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())

  function setNodeRunning(nodeId: string, running: boolean) {
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
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
      x: Number(snapshot.x ?? 0),
      y: Number(snapshot.y ?? 0),
      width: Number.isFinite(width) && width > 0 ? width : 280,
    }
  }

  function commitNodeMutation(mutator: (items: Node[]) => Node[]) {
    const historyBefore = options.captureHistory()
    const nextNodes = mutator(options.nodes.value)
    options.commitNodes(nextNodes)
    options.pushHistory(historyBefore)
    options.markDirty()
    options.refreshSnapshot()
  }

  function appendImageResultNode(source: ImageDirectActionSource, url: string, ratio: number) {
    const imageNode = createImageNodeAt({ x: source.x + source.width + 48, y: source.y })
    imageNode.width = source.width
    imageNode.sourceImageUrl = url
    imageNode.sourceImageRatio = ratio > 0 ? ratio : source.ratio
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
    if (runningNodeIds.value.has(nodeId)) return
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    setNodeRunning(nodeId, true)
    try {
      const urls =
        action === 'to-vector'
          ? await runToVector(source)
          : await runSubmitAndWait(source, action, runOptions?.strength)
      const firstUrl = urls.find((url) => typeof url === 'string' && url.trim())?.trim()
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      const ratio = await resolveResultRatio(firstUrl, source.ratio)
      appendImageResultNode(source, firstUrl, ratio)
      options.onSuccess('处理完成')
    } catch (error) {
      const msg = error instanceof Error ? error.message : '执行失败'
      options.onWarning(msg)
    } finally {
      setNodeRunning(nodeId, false)
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    runImageDirectAction,
  }
}
