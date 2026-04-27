import { onUnmounted, ref, type Ref } from 'vue'
import type { CanvasImageItem, CanvasPoint, ImageNodeItem } from '../types/canvasNodes'
import { isCanvasImagePending } from '../types/canvasNodes'
import { sanitizeApiErrorMessage, sanitizeTaskErrorMessage } from '../../../shared/utils/errorMessages'

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
  suppressDefaultAppend?: boolean
  onSubmitted?: (taskId: string) => void
  onCompleted?: (urls: string[]) => void
  onFailed?: () => void
}

interface UseGenerationPipelineOptions<TSubmitPayload extends object> {
  generating: Ref<boolean>
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  imageNodes: Ref<ImageNodeItem[]>
  activeImageNodeId: Ref<number | null>
  preferImageOutputNode?: boolean
  message: MessageApiLike
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  submitTask: (payload: TSubmitPayload) => Promise<TaskSubmitResponseLike>
  getTask: (taskId: string) => Promise<TaskGetResponseLike>
  onCanvasImagesChanged?: () => void
  onImageNodeOutputsChanged?: (node: ImageNodeItem) => void
}

const GENERATED_CANVAS_IMAGE_WIDTH = 300
const GENERATED_CANVAS_IMAGE_STEP_X = 170
const GENERATED_CANVAS_IMAGE_SOURCE_OFFSET_X = 50
const GENERATED_CANVAS_IMAGE_SOURCE_OFFSET_Y = 26

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
  const activeTaskIds = ref<string[]>([])
  const pollTimers = new Map<string, number>()

  function syncCurrentTaskId() {
    currentTaskId.value = activeTaskIds.value[activeTaskIds.value.length - 1] ?? null
  }

  function setTaskActive(taskId: string, active: boolean) {
    if (active) {
      if (!activeTaskIds.value.includes(taskId)) {
        activeTaskIds.value = [...activeTaskIds.value, taskId]
      }
    } else if (activeTaskIds.value.includes(taskId)) {
      activeTaskIds.value = activeTaskIds.value.filter(id => id !== taskId)
    }
    syncCurrentTaskId()
    options.generating.value = activeTaskIds.value.length > 0
  }

  function isTaskActive(taskId: string) {
    return activeTaskIds.value.includes(taskId)
  }

  function stopTaskPolling(taskId?: string) {
    if (taskId) {
      const timer = pollTimers.get(taskId)
      if (timer != null) {
        clearTimeout(timer)
        pollTimers.delete(taskId)
      }
      setTaskActive(taskId, false)
      return
    }

    pollTimers.forEach((timer) => {
      clearTimeout(timer)
    })
    pollTimers.clear()
    activeTaskIds.value = []
    syncCurrentTaskId()
    options.generating.value = false
  }

  function resolvePreferredImageOutputNode(providedNodeId?: number | null): ImageNodeItem | null {
    const preferImageOutputNode = options.preferImageOutputNode ?? true
    if (providedNodeId !== null && providedNodeId !== undefined) {
      const provided = options.imageNodes.value.find(node => node.id === providedNodeId)
      if (provided) return provided
    }
    if (!preferImageOutputNode) return null
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
        options.onImageNodeOutputsChanged?.(targetNode)
      })
    })
    options.onImageNodeOutputsChanged?.(node)
  }

  function syncCanvasImageRatio(itemId: number, url: string) {
    void options.loadImageRatioFromUrl(url).then((ratio) => {
      if (!ratio) return
      const target = options.messages.value.find(item => item.id === itemId)
      if (!target || target.url !== url) return
      target.height_ratio = ratio
      options.onCanvasImagesChanged?.()
    })
  }

  function buildGeneratedCanvasPlacements(source: CanvasImageItem | undefined, count: number): CanvasPoint[] {
    const baseX = source
      ? source.x - GENERATED_CANVAS_IMAGE_WIDTH - GENERATED_CANVAS_IMAGE_SOURCE_OFFSET_X
      : -((count - 1) * GENERATED_CANVAS_IMAGE_STEP_X) / 2
    const baseY = source ? source.y + GENERATED_CANVAS_IMAGE_SOURCE_OFFSET_Y : 0

    return Array.from({ length: count }, (_, index) => ({
      x: source ? baseX - index * GENERATED_CANVAS_IMAGE_STEP_X : baseX + index * GENERATED_CANVAS_IMAGE_STEP_X,
      y: baseY + index * 22,
    }))
  }

  function createPendingGeneratedImages(config: {
    count?: number
    source?: CanvasImageItem
    outputNodeId?: number | null
    heightRatios?: number[]
  }): CanvasImageItem[] {
    const outputNode = resolvePreferredImageOutputNode(config.outputNodeId)
    if (outputNode) return []

    const count = Math.max(1, config.count ?? 1)
    const placements = buildGeneratedCanvasPlacements(config.source, count)
    const items = placements.map((point, index) => {
      const item = options.createCanvasImageAt(point, '', config.heightRatios?.[index])
      item.width = GENERATED_CANVAS_IMAGE_WIDTH
      item.renderState = 'pending'
      return item
    })

    if (items.length > 0) {
      options.messages.value.push(...items)
      options.selectedIds.value = items.map(item => item.id)
      options.onCanvasImagesChanged?.()
    }

    return items
  }

  function removePendingGeneratedImages(pendingItems: CanvasImageItem[]) {
    const ids = new Set(pendingItems.map(item => item.id))
    if (ids.size === 0) return
    options.messages.value = options.messages.value.filter(item => !ids.has(item.id))
    if (options.selectedIds.value.some(id => ids.has(id))) {
      options.selectedIds.value = options.selectedIds.value.filter(id => !ids.has(id))
    }
    options.onCanvasImagesChanged?.()
  }

  function replacePendingGeneratedImages(
    urls: string[],
    pendingItems: CanvasImageItem[],
    source?: CanvasImageItem,
    outputNodeId?: number | null,
  ) {
    const outputNode = resolvePreferredImageOutputNode(outputNodeId)
    if (outputNode) {
      appendUrlsToImageNodeOutputs(urls, outputNode)
      return
    }
    if (urls.length === 0) {
      removePendingGeneratedImages(pendingItems)
      return
    }

    const livePending = pendingItems
      .map(item => options.messages.value.find(candidate => candidate.id === item.id) ?? null)
      .filter((item): item is CanvasImageItem => !!item && isCanvasImagePending(item))

    if (livePending.length === 0) {
      appendGeneratedImages(urls, source, outputNodeId)
      return
    }

    const placements = buildGeneratedCanvasPlacements(source, urls.length)
    const nextSelectedIds: number[] = []

    urls.forEach((url, index) => {
      const point = placements[index]
      const target = livePending[index]
      if (target && point) {
        target.url = url
        target.x = point.x
        target.y = point.y
        target.width = GENERATED_CANVAS_IMAGE_WIDTH
        target.renderState = 'ready'
        syncCanvasImageRatio(target.id, url)
        nextSelectedIds.push(target.id)
        return
      }

      if (!point) return
      const item = options.createCanvasImageAt(point, url)
      item.width = GENERATED_CANVAS_IMAGE_WIDTH
      item.renderState = 'ready'
      options.messages.value.push(item)
      syncCanvasImageRatio(item.id, url)
      nextSelectedIds.push(item.id)
    })

    if (livePending.length > urls.length) {
      const staleIds = new Set(livePending.slice(urls.length).map(item => item.id))
      options.messages.value = options.messages.value.filter(item => !staleIds.has(item.id))
    }

    if (nextSelectedIds.length > 0) {
      options.selectedIds.value = nextSelectedIds
    }
    options.onCanvasImagesChanged?.()
  }

  function appendGeneratedImages(urls: string[], source?: CanvasImageItem, outputNodeId?: number | null) {
    if (!urls.length) return
    const outputNode = resolvePreferredImageOutputNode(outputNodeId)
    appendUrlsToImageNodeOutputs(urls, outputNode)
    if (outputNode) return

    const placements = buildGeneratedCanvasPlacements(source, urls.length)
    const newIds: number[] = []

    urls.forEach((url, index) => {
      const point = placements[index]
      if (!point) return
      const item = options.createCanvasImageAt(point, url)
      item.width = GENERATED_CANVAS_IMAGE_WIDTH
      item.renderState = 'ready'
      options.messages.value.push(item)
      syncCanvasImageRatio(item.id, url)
      newIds.push(item.id)
    })

    if (newIds.length > 0) {
      options.selectedIds.value = newIds
    }
    options.onCanvasImagesChanged?.()
  }

  function startTaskPolling(
    taskId: string,
    source?: CanvasImageItem,
    outputNodeId?: number | null,
    suppressDefaultAppend?: boolean,
    onCompleted?: (urls: string[]) => void,
    onFailed?: () => void,
  ) {
    stopTaskPolling(taskId)
    setTaskActive(taskId, true)

    const check = async () => {
      try {
        const res = await options.getTask(taskId)
        if (!isTaskActive(taskId)) return
        if (res.code === 0 && res.data?.task) {
          const task = res.data.task
          if (task.status === 'completed') {
            const urls = extractImageUrlsFromTask(task)
            if (urls.length > 0) {
              onCompleted?.(urls)
              if (!suppressDefaultAppend) {
                appendGeneratedImages(urls, source, outputNodeId)
              }
              options.message.success('生成完成')
            } else {
              onFailed?.()
              options.message.warning('任务完成，但未返回图片')
            }
            stopTaskPolling(taskId)
            return
          }
          if (task.status === 'failed') {
            options.message.error(sanitizeTaskErrorMessage(task.error_msg, '生成失败'))
            onFailed?.()
            stopTaskPolling(taskId)
            return
          }
        }
        if (!isTaskActive(taskId)) return
        const timer = window.setTimeout(check, 2000)
        pollTimers.set(taskId, timer)
      } catch {
        if (!isTaskActive(taskId)) return
        const timer = window.setTimeout(check, 3000)
        pollTimers.set(taskId, timer)
      }
    }

    void check()
  }

  async function submitTaskAndPoll(submitOptions: SubmitAndPollOptions<TSubmitPayload>): Promise<string | null> {
    try {
      const submitRes = await options.submitTask(submitOptions.request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        options.message.error(
          sanitizeApiErrorMessage(submitRes.msg, submitOptions.failureMessage || 'Failed to submit task'),
        )
        submitOptions.onFailed?.()
        options.generating.value = activeTaskIds.value.length > 0
        return null
      }

      const taskId = submitRes.data.task_id
      currentTaskId.value = taskId
      options.message.success(submitOptions.successMessage)
      submitOptions.onSubmitted?.(taskId)
      startTaskPolling(
        taskId,
        submitOptions.source,
        submitOptions.outputNodeId,
        submitOptions.suppressDefaultAppend,
        submitOptions.onCompleted,
        submitOptions.onFailed,
      )
      return taskId
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      const errorMessage = maybeError?.msg || maybeError?.message
      options.message.error(
        sanitizeApiErrorMessage(errorMessage, submitOptions.failureMessage || 'Failed to submit task'),
      )
      submitOptions.onFailed?.()
      options.generating.value = activeTaskIds.value.length > 0
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
    createPendingGeneratedImages,
    replacePendingGeneratedImages,
    removePendingGeneratedImages,
    extractImageUrls: extractImageUrlsFromTask,
    startTaskPolling,
    submitTaskAndPoll,
  }
}
