import { watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'
import { normalizeTaskImageUrls, resolveTaskImageRatio } from '../utils/flowImageDirectActionTaskResult'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'

type PendingImageNodeItem = ImageNodeItem & {
  pendingTaskId?: string
}

type UsePendingImageTaskResumeOptions = {
  nodes: Ref<Node[]>
  runningNodeIds: Ref<Set<string>>
  setNodeRunning: (nodeId: string, running: boolean) => void
  waitTaskResult: (taskId: string) => Promise<string[]>
  updatePendingImageResultNode: (nodeId: string, url: string, ratio: number) => void
  removePendingImageResultNode: (nodeId: string) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
}

export function usePendingImageTaskResume(options: UsePendingImageTaskResumeOptions) {
  const resumedTaskIds = new Set<string>()

  async function resumePendingImageResultNode(nodeId: string, taskId: string, fallbackRatio: number) {
    if (options.runningNodeIds.value.has(nodeId) || resumedTaskIds.has(taskId)) return
    resumedTaskIds.add(taskId)
    options.setNodeRunning(nodeId, true)
    try {
      const urls = normalizeTaskImageUrls(await options.waitTaskResult(taskId))
      const url = urls[0]
      if (!url) throw new Error('任务完成但未返回图片')
      const ratio = await resolveTaskImageRatio(url, fallbackRatio, loadImageSizeFromUrl)
      options.updatePendingImageResultNode(nodeId, url, ratio)
      options.onSuccess('生成完成')
    } catch (error) {
      options.removePendingImageResultNode(nodeId)
      const msg = error instanceof Error ? error.message : '生成失败'
      options.onWarning(msg)
    } finally {
      options.setNodeRunning(nodeId, false)
      resumedTaskIds.delete(taskId)
    }
  }

  watch(
    () =>
      options.nodes.value
        .map((node) => {
          const snapshot = node.data?.snapshotNode as PendingImageNodeItem | undefined
          return snapshot?.outputStatus === 'running' && snapshot.pendingTaskId
            ? `${node.id}:${snapshot.pendingTaskId}`
            : ''
        })
        .filter(Boolean)
        .join('|'),
    () => {
      options.nodes.value.forEach((node) => {
        if (node.data?.kind !== 'image') return
        const snapshot = node.data.snapshotNode as PendingImageNodeItem | undefined
        if (!snapshot || snapshot.outputStatus !== 'running') return
        const taskId = `${snapshot.pendingTaskId ?? ''}`.trim()
        if (!taskId) return
        const fallbackRatio = Number(snapshot.sourceImageRatio)
        void resumePendingImageResultNode(
          node.id,
          taskId,
          Number.isFinite(fallbackRatio) && fallbackRatio > 0 ? fallbackRatio : 1,
        )
      })
    },
    { immediate: true },
  )
}
