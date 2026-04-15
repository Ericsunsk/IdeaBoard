import { onUnmounted, ref, type Ref } from 'vue'
import type { CanvasImageItem, CanvasPoint, ImageNodeItem } from '../types/canvasNodes'

interface MessageApiLike {
  success(content: string): unknown
  warning(content: string): unknown
  error(content: string): unknown
}

interface TaskRecordLike {
  status?: string
  result?: string
  error_msg?: string
}

interface TaskGetResponseLike {
  code?: number
  data?: {
    task?: TaskRecordLike
  }
}

interface TaskSubmitResponseLike {
  code?: number
  msg?: string
  data?: {
    task_id?: string
  }
}

interface SubmitAndPollOptions<TSubmitPayload extends object> {
  request: TSubmitPayload
  successMessage: string
  failureMessage?: string
  source?: CanvasImageItem
  outputNodeId?: number | null
  onSubmitted?: (taskId: string) => void
}

interface UseGenerationPipelineOptions<TSubmitPayload extends object> {
  generating: Ref<boolean>
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  imageNodes: Ref<ImageNodeItem[]>
  activeImageNodeId: Ref<number | null>
  message: MessageApiLike
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  submitTask: (payload: TSubmitPayload) => Promise<TaskSubmitResponseLike>
  getTask: (taskId: string) => Promise<TaskGetResponseLike>
}

function extractImageUrlsFromTask(task: TaskRecordLike | null | undefined): string[] {
  if (!task?.result) return []
  try {
    const parsed = JSON.parse(task.result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls.filter((url: unknown): url is string => typeof url === 'string' && url.length > 0)
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url) {
      return [parsed.image_url]
    }
    return []
  } catch {
    return []
  }
}

export function useGenerationPipeline<TSubmitPayload extends object>(
  options: UseGenerationPipelineOptions<TSubmitPayload>,
) {
  const currentTaskId = ref<string | null>(null)
  let pollTimer: number | null = null

  function stopTaskPolling() {
    if (!pollTimer) return
    clearTimeout(pollTimer)
    pollTimer = null
  }

  function resolvePreferredImageOutputNode(providedNodeId?: number | null): ImageNodeItem | null {
    if (providedNodeId !== null && providedNodeId !== undefined) {
      const provided = options.imageNodes.value.find(node => node.id === providedNodeId)
      if (provided) return provided
    }
    if (options.activeImageNodeId.value !== null) {
      const active = options.imageNodes.value.find(node => node.id === options.activeImageNodeId.value)
      if (active) return active
    }
    return options.imageNodes.value[options.imageNodes.value.length - 1] ?? null
  }

  function appendUrlsToImageNodeOutputs(urls: string[], node: ImageNodeItem | null) {
    if (!node || urls.length === 0) return
    const startIndex = node.outputImages.length
    urls.forEach((url) => {
      node.outputImages.push({ url, ratio: 1 })
    })
    urls.forEach((url, offset) => {
      const outputIndex = startIndex + offset
      void options.loadImageRatioFromUrl(url).then((ratio) => {
        if (!ratio) return
        const targetNode = options.imageNodes.value.find(item => item.id === node.id)
        if (!targetNode) return
        const output = targetNode.outputImages[outputIndex]
        if (!output || output.url !== url) return
        output.ratio = ratio
      })
    })
  }

  function appendGeneratedImages(urls: string[], source?: CanvasImageItem, outputNodeId?: number | null) {
    if (!urls.length) return
    const outputNode = resolvePreferredImageOutputNode(outputNodeId)
    appendUrlsToImageNodeOutputs(urls, outputNode)

    const generatedWidth = 300
    const stepX = 170
    const baseX = source
      ? source.x - generatedWidth - 50
      : -((urls.length - 1) * stepX) / 2
    const baseY = source ? source.y + 26 : 0
    const newIds: number[] = []

    urls.forEach((url, index) => {
      const item = options.createCanvasImageAt(
        {
          x: source ? baseX - index * stepX : baseX + index * stepX,
          y: baseY + index * 22,
        },
        url,
      )
      item.width = generatedWidth
      options.messages.value.push(item)
      newIds.push(item.id)
    })

    if (newIds.length > 0) {
      options.selectedIds.value = newIds
    }
  }

  function startTaskPolling(taskId: string, source?: CanvasImageItem, outputNodeId?: number | null) {
    stopTaskPolling()

    const check = async () => {
      try {
        const res = await options.getTask(taskId)
        if (res.code === 0 && res.data?.task) {
          const task = res.data.task
          if (task.status === 'completed') {
            const urls = extractImageUrlsFromTask(task)
            if (urls.length > 0) {
              appendGeneratedImages(urls, source, outputNodeId)
              options.message.success('生成完成')
            } else {
              options.message.warning('任务完成，但未返回图片')
            }
            currentTaskId.value = null
            options.generating.value = false
            return
          }
          if (task.status === 'failed') {
            options.message.error(task.error_msg || '生成失败')
            currentTaskId.value = null
            options.generating.value = false
            return
          }
        }
        pollTimer = window.setTimeout(check, 2000)
      } catch {
        pollTimer = window.setTimeout(check, 3000)
      }
    }

    void check()
  }

  async function submitTaskAndPoll(submitOptions: SubmitAndPollOptions<TSubmitPayload>): Promise<string | null> {
    try {
      const submitRes = await options.submitTask(submitOptions.request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        options.message.error(submitRes.msg || submitOptions.failureMessage || 'Submit failed')
        options.generating.value = false
        return null
      }

      const taskId = submitRes.data.task_id
      currentTaskId.value = taskId
      options.message.success(submitOptions.successMessage)
      submitOptions.onSubmitted?.(taskId)
      startTaskPolling(taskId, submitOptions.source, submitOptions.outputNodeId)
      return taskId
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || submitOptions.failureMessage || 'Failed to submit task')
      options.generating.value = false
      return null
    }
  }

  onUnmounted(() => {
    stopTaskPolling()
  })

  return {
    currentTaskId,
    stopTaskPolling,
    resolvePreferredImageOutputNode,
    appendUrlsToImageNodeOutputs,
    appendGeneratedImages,
    extractImageUrls: extractImageUrlsFromTask,
    startTaskPolling,
    submitTaskAndPoll,
  }
}
