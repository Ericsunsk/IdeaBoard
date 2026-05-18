import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import {
  assertDirectActionSubmitReady,
  assertPromptImageSubmitReady,
  runToVector,
  submitDirectActionTask,
  submitPromptImageGenerationTask,
  waitExistingOrLiveTaskResult,
} from '../services/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'
import { usePendingImageResultNodes } from './usePendingImageResultNodes'
import { usePendingImageTaskResume } from './usePendingImageTaskResume'
import {
  normalizeTaskImageUrls,
  pickFirstValidUrl,
  resolveTaskImageRatio,
} from '../utils/flowImageDirectActionTaskResult'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import type { FlowTransactions } from './useFlowTransactions'

type DirectActionKey = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'
type RunImageDirectActionOptions = {
  strength?: number
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
  refreshNodeInternals?: (nodeIds: string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const {
    appendPendingImageResultNode,
    appendPendingImageResultNodes,
    updatePendingImageResultNode,
    updatePendingImageTaskMeta,
    removePendingImageResultNode,
  } = usePendingImageResultNodes(options)

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
    return resolveImageDirectActionSource(node, node.data.snapshotNode as ImageNodeItem, {
      requireUrl: true,
    })
  }

  usePendingImageTaskResume({
    nodes: options.nodes,
    runningNodeIds,
    setNodeRunning,
    waitTaskResult: waitExistingOrLiveTaskResult,
    updatePendingImageResultNode,
    removePendingImageResultNode,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })

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
      const taskId =
        action === 'to-vector'
          ? ''
          : await submitDirectActionTask(
              source,
              action,
              loadImageSizeFromUrl,
              runOptions?.strength,
            )
      if (taskId) {
        updatePendingImageTaskMeta(resultNodeId, {
          pendingTaskId: taskId,
          pendingSourceNodeId: nodeId,
          pendingPrompt: '',
        })
      }
      const urls =
        action === 'to-vector' ? await runToVector(source) : await waitExistingOrLiveTaskResult(taskId)
      const firstUrl = pickFirstValidUrl(urls)
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      const ratio = await resolveTaskImageRatio(firstUrl, source.ratio, loadImageSizeFromUrl)
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

  async function runImagePromptGeneration(nodeId: string, prompt: string, outputCount: number) {
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

    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    let resultNodeIds: string[] = []
    try {
      await assertPromptImageSubmitReady()
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount)
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      const results = await Promise.allSettled(
        resultNodeIds.map(async (resultNodeId) => {
          const taskId = await submitPromptImageGenerationTask(
            source,
            normalizedPrompt,
            loadImageSizeFromUrl,
          )
          updatePendingImageTaskMeta(resultNodeId, {
            pendingTaskId: taskId,
            pendingSourceNodeId: nodeId,
            pendingPrompt: normalizedPrompt,
          })
          const urls = normalizeTaskImageUrls(await waitExistingOrLiveTaskResult(taskId))
          const url = urls[0]
          if (!url) throw new Error('任务完成但未返回图片')
          const ratio = await resolveTaskImageRatio(url, source.ratio, loadImageSizeFromUrl)
          return { resultNodeId, url, ratio }
        }),
      )
      let successCount = 0
      let firstFailure: unknown = null
      results.forEach((result) => {
        if (result.status === 'fulfilled') {
          successCount += 1
          updatePendingImageResultNode(
            result.value.resultNodeId,
            result.value.url,
            result.value.ratio,
          )
          return
        }
        firstFailure ??= result.reason
      })
      resultNodeIds.forEach((resultNodeId, index) => {
        if (results[index]?.status !== 'fulfilled') {
          removePendingImageResultNode(resultNodeId)
        }
      })
      if (successCount === 0) {
        throw firstFailure instanceof Error ? firstFailure : new Error('生成失败')
      }
      if (successCount < resolvedOutputCount) {
        options.onWarning(`生成完成 ${successCount}/${resolvedOutputCount} 张，部分任务失败`)
      } else {
        options.onSuccess(successCount > 1 ? `生成完成（${successCount} 张）` : '生成完成')
      }
    } catch (error) {
      for (const resultNodeId of resultNodeIds) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '生成失败'
      options.onWarning(msg)
    } finally {
      for (const resultNodeId of resultNodeIds) {
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
