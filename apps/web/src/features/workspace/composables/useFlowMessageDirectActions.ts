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
import { createCanvasImageItemAt } from '../utils/canvasNodeFactory'
import type { CanvasImageItem, ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { waitTaskResult } from '../services/flowTaskExecutorUtils'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'

type DirectActionKey = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'
type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }
type RunMessageDirectActionOptions = {
  strength?: number
}

type UseFlowMessageDirectActionsOptions = {
  nodes: Ref<Node[]>
  captureHistory: () => FlowHistorySnapshot
  pushHistory: (snapshot: FlowHistorySnapshot) => void
  markDirty: () => void
  refreshSnapshot: () => void
  commitNodes: (nextNodes: Node[]) => void
  updateSnapshotNodeSilently: (
    nodeId: string,
    patcher: (snapshotNode: Record<string, unknown>) => Record<string, unknown>,
  ) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
}

const STYLE_INNOVATE_DEFAULT_STRENGTH = 50


export function useFlowMessageDirectActions(options: UseFlowMessageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())

  function getDirectActionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || (target.data?.kind !== 'message' && target.data?.kind !== 'image')) return null
    return target as Node<FlowNodeData>
  }

  function readMessageSnapshot(nodeId: string): CanvasImageItem | null {
    const node = getDirectActionNodeById(nodeId)
    if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    if (node.data.kind === 'image') {
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
      return {
        id: snapshot.id,
        type: 'image',
        url,
        x: snapshot.x,
        y: snapshot.y,
        width: snapshot.width,
        height_ratio: ratio,
      }
    }
    return node.data.snapshotNode as CanvasImageItem
  }

  function commitNodeMutation(mutator: (items: Node[]) => Node[]) {
    const historyBefore = options.captureHistory()
    const nextNodes = mutator(options.nodes.value)
    options.commitNodes(nextNodes)
    options.pushHistory(historyBefore)
    options.markDirty()
    options.refreshSnapshot()
  }

  function appendPendingMessageNode(source: CanvasImageItem): string {
    const sourceWidth = Number(source.width)
    const width = Number.isFinite(sourceWidth) && sourceWidth > 0 ? sourceWidth : 300
    const pendingItem = createCanvasImageItemAt(
      { x: source.x + width + 48, y: source.y },
      source.url,
      source.height_ratio,
    )
    pendingItem.width = width
    pendingItem.renderState = 'pending'

    const nextNode: Node<FlowNodeData> = {
      id: `message-${pendingItem.id}`,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('message'),
      position: { x: pendingItem.x, y: pendingItem.y },
      data: {
        kind: 'message',
        title: `Message #${pendingItem.id}`,
        summary: 'Canvas message image',
        snapshotNode: pendingItem,
      },
    }

    commitNodeMutation((items) => [...items, nextNode as Node])
    return nextNode.id
  }


  function removeMessageNode(nodeId: string) {
    commitNodeMutation((items) => items.filter((node) => node.id !== nodeId))
  }

  function replacePendingNodeResultUrl(nodeId: string, url: string) {
    commitNodeMutation((items) =>
      items.map((node) => {
        if (node.id !== nodeId || node.data?.kind !== 'message') return node
        const snapshotNode = node.data.snapshotNode
        if (!snapshotNode || typeof snapshotNode !== 'object') return node
        const prev = snapshotNode as CanvasImageItem
        const nextSnapshot: CanvasImageItem = {
          ...prev,
          url,
          renderState: 'ready',
        }
        return {
          ...node,
          data: {
            ...node.data,
            snapshotNode: nextSnapshot,
          },
        }
      }),
    )
    // Backfill real image ratio asynchronously without creating extra history
    loadImageSizeFromUrl(url)
      .then((size) => {
        if (size && size.width > 0 && size.height > 0) {
          const ratio = size.height / size.width
          options.updateSnapshotNodeSilently(nodeId, (prev) => ({
            ...prev,
            height_ratio: ratio,
          }))
        }
      })
      .catch(() => {})
  }

  async function runSubmitAndWait(
    source: CanvasImageItem,
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


  async function runToVector(source: CanvasImageItem): Promise<string[]> {
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

  async function runMessageDirectAction(
    nodeId: string,
    action: DirectActionKey,
    runOptions?: RunMessageDirectActionOptions,
  ) {
    if (runningNodeIds.value.has(nodeId)) return
    const source = readMessageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    runningNodeIds.value.add(nodeId)
    const pendingNodeId = appendPendingMessageNode(source)
    try {
      const urls =
        action === 'to-vector'
          ? await runToVector(source)
          : await runSubmitAndWait(source, action, runOptions?.strength)
      const firstUrl = urls.find((url) => typeof url === 'string' && url.trim())
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      replacePendingNodeResultUrl(pendingNodeId, firstUrl)
      options.onSuccess('处理完成')
    } catch (error) {
      removeMessageNode(pendingNodeId)
      const msg = error instanceof Error ? error.message : '执行失败'
      options.onWarning(msg)
    } finally {
      runningNodeIds.value.delete(nodeId)
    }
  }


  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    runMessageDirectAction,
  }
}
