import type { ComputedRef, Ref } from 'vue'
import type { CanvasImageItem } from '../types/canvasNodes'
import type { SubmitTaskData } from '../../../shared/api/index'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  buildExecutionTaskRequest,
  EXECUTION_DIRECT_ACTION_META,
  EXECUTION_REMOVE_BG_META,
} from '../config/executionContract'

export type SegType = 'common' | 'commodity' | 'body'
export type CanvasActionKey = 'remove-bg' | 'style-innovate' | 'sketch-to-design' | 'to-vector'

interface MessageLoadingHandle {
  destroy: () => void
}

interface MessageApiLike {
  success(content: string): unknown
  warning(content: string): unknown
  error(content: string): unknown
  loading(
    content: string,
    options?: {
      duration?: number
      keepAliveOnHover?: boolean
    },
  ): MessageLoadingHandle
}

interface SubmitTaskAndPollOptions {
  request: SubmitTaskData
  source?: CanvasImageItem
  successMessage: string
  failureMessage: string
  suppressDefaultAppend?: boolean
  onSubmitted?: () => void
  onCompleted?: (urls: string[]) => void
  onFailed?: () => void
}

interface UseCanvasDirectActionsOptions {
  generating: Ref<boolean>
  selectedIds: Ref<number[]>
  activeImageActionId: Ref<number | null>
  showStyleStrengthControl: Ref<boolean>
  hasDirectPrompt: ComputedRef<boolean>
  showSettings: Ref<boolean>
  selectedRatio: Ref<string>
  selectedQuality: Ref<'1k' | '2k' | '4k'>
  userPrompt: Ref<string>
  messages: Ref<CanvasImageItem[]>
  styleInnovateStrength: Ref<number>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
  defaultSegType: SegType
  message: MessageApiLike
  submitTaskAndPoll: (options: SubmitTaskAndPollOptions) => Promise<string | null>
  appendGeneratedImages: (urls: string[], source?: CanvasImageItem) => void
  createPendingGeneratedImages: (config: {
    count?: number
    source?: CanvasImageItem
    heightRatios?: number[]
  }) => CanvasImageItem[]
  replacePendingGeneratedImages: (
    urls: string[],
    pendingItems: CanvasImageItem[],
    source?: CanvasImageItem,
  ) => void
  removePendingGeneratedImages: (pendingItems: CanvasImageItem[]) => void
  segmentImage: (payload: { image_url: string; seg_type: SegType }) => Promise<any>
  saveSegmentResult: (payload: { result_url: string; seg_type: SegType; feature: string }) => Promise<any>
  vectorizeImage: (payload: {
    image_url: string
    threshold: number
    turd_size: number
    alpha_max: number
    opt_tolerance: number
    turn_policy: string
    invert: boolean
    color_count: number
  }) => Promise<any>
}

export function resolveDirectPromptText(rawPrompt: string, fallback = ''): string {
  return rawPrompt.trim() || fallback
}

export function toSafeFileName(rawName: string): string {
  const decoded = rawName ? decodeURIComponent(rawName) : ''
  const cleaned = decoded.replace(/[\\/:*?"<>|]+/g, '_').trim()
  if (!cleaned) return `export-${Date.now()}.png`
  if (/\.[a-z0-9]+$/i.test(cleaned)) return cleaned
  return `${cleaned}.png`
}

export function buildExportFileName(url: string, index: number, total: number, timestamp: number): string {
  const fromUrl = String(url).split('/').pop()?.split('?')[0] || ''
  const safeName = toSafeFileName(fromUrl)
  if (total > 1) {
    return `${String(index + 1).padStart(2, '0')}-${timestamp}-${safeName}`
  }
  return safeName
}

function parseAspectRatio(rawRatio: string): number {
  const [width, height] = rawRatio.split(':').map(part => Number(part))
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return 1
  }
  return height / width
}

export function useCanvasDirectActions(options: UseCanvasDirectActionsOptions) {
  let exportLoading: MessageLoadingHandle | null = null

  async function handlePictureGenerate() {
    const promptText = resolveDirectPromptText(options.userPrompt.value)
    if (!promptText) return
    options.generating.value = true
    let pendingItems: CanvasImageItem[] = []
    await options.submitTaskAndPoll({
      request: buildExecutionTaskRequest({
        prompt: promptText,
        feature: EXECUTION_DIRECT_ACTION_META.patternGen.feature,
        params: {
          ratio: options.selectedRatio.value,
          quality: options.selectedQuality.value,
        },
      }),
      successMessage: '任务已提交',
      failureMessage: '提交失败',
      suppressDefaultAppend: true,
      onCompleted: (urls) => {
        options.replacePendingGeneratedImages(urls, pendingItems)
      },
      onFailed: () => {
        options.removePendingGeneratedImages(pendingItems)
      },
      onSubmitted: () => {
        pendingItems = options.createPendingGeneratedImages({
          heightRatios: [parseAspectRatio(options.selectedRatio.value)],
        })
        options.userPrompt.value = ''
        options.showSettings.value = false
      },
    })
  }

  async function handleRemoveBgGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const startTime = Date.now()
    const pendingItems = options.createPendingGeneratedImages({
      source,
      heightRatios: [source.height_ratio || 1],
    })
    try {
      const segmentRes: any = await options.segmentImage({
        image_url: source.url,
        seg_type: options.defaultSegType,
      })
      if (segmentRes.code !== 0 || !segmentRes.data?.result_url) {
        options.removePendingGeneratedImages(pendingItems)
        options.message.error(sanitizeApiErrorMessage(segmentRes.msg, 'Segmentation failed'))
        return
      }

      const saveRes: any = await options.saveSegmentResult({
        result_url: segmentRes.data.result_url,
        seg_type: options.defaultSegType,
        feature: EXECUTION_REMOVE_BG_META.feature,
      })
      if (saveRes.code !== 0 || !saveRes.data?.image_url) {
        options.removePendingGeneratedImages(pendingItems)
        options.message.error(sanitizeApiErrorMessage(saveRes.msg, 'Save result failed'))
        return
      }

      options.replacePendingGeneratedImages([saveRes.data.image_url], pendingItems, source)
      options.message.success(`Background removed in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
    } catch (error: any) {
      options.removePendingGeneratedImages(pendingItems)
      options.message.error(sanitizeApiErrorMessage(error?.msg || error?.message, 'Failed to remove background'))
    } finally {
      options.generating.value = false
    }
  }

  async function handleSketchToDesignGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const desc = resolveDirectPromptText(
      options.userPrompt.value,
      EXECUTION_DIRECT_ACTION_META.sketchToDesign.fallbackPrompt,
    )
    let pendingItems: CanvasImageItem[] = []
    await options.submitTaskAndPoll({
      request: buildExecutionTaskRequest({
        prompt: `${EXECUTION_DIRECT_ACTION_META.sketchToDesign.promptPrefix} ${desc}`,
        feature: EXECUTION_DIRECT_ACTION_META.sketchToDesign.feature,
        imageUrls: [source.url],
      }),
      source,
      successMessage: 'Task submitted',
      failureMessage: 'Failed to submit task',
      suppressDefaultAppend: true,
      onCompleted: (urls) => {
        options.replacePendingGeneratedImages(urls, pendingItems, source)
      },
      onFailed: () => {
        options.removePendingGeneratedImages(pendingItems)
      },
      onSubmitted: () => {
        pendingItems = options.createPendingGeneratedImages({
          source,
          heightRatios: [source.height_ratio || 1],
        })
        options.userPrompt.value = ''
      },
    })
  }

  async function handleStyleInnovateGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const desc = resolveDirectPromptText(
      options.userPrompt.value,
      EXECUTION_DIRECT_ACTION_META.styleInnovate.fallbackPrompt,
    )
    const strength = options.styleInnovateStrength.value
    const imageSize = await options.loadImageSizeFromUrl(source.url)
    let pendingItems: CanvasImageItem[] = []
    await options.submitTaskAndPoll({
      request: buildExecutionTaskRequest({
        prompt: `${EXECUTION_DIRECT_ACTION_META.styleInnovate.promptPrefix} 创新强度：${strength}%，${desc}`,
        feature: EXECUTION_DIRECT_ACTION_META.styleInnovate.feature,
        imageUrls: [source.url],
        params: {
          original_image_url: source.url,
          ...(imageSize ? {
            custom_width: imageSize.width,
            custom_height: imageSize.height,
            original_image_width: imageSize.width,
            original_image_height: imageSize.height,
          } : {}),
          variation_strength: strength,
          strength,
        },
      }),
      source,
      successMessage: 'Task submitted',
      failureMessage: 'Failed to submit task',
      suppressDefaultAppend: true,
      onCompleted: (urls) => {
        options.replacePendingGeneratedImages(urls, pendingItems, source)
      },
      onFailed: () => {
        options.removePendingGeneratedImages(pendingItems)
      },
      onSubmitted: () => {
        pendingItems = options.createPendingGeneratedImages({
          source,
          heightRatios: [source.height_ratio || 1],
        })
        options.userPrompt.value = ''
      },
    })
  }

  async function handleToVectorGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const startTime = Date.now()
    const pendingItems = options.createPendingGeneratedImages({
      source,
      heightRatios: [source.height_ratio || 1],
    })
    try {
      const res: any = await options.vectorizeImage({
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
        options.removePendingGeneratedImages(pendingItems)
        options.message.error(sanitizeApiErrorMessage(res.msg, 'Vectorize failed'))
        return
      }

      options.replacePendingGeneratedImages([res.data.svg_url], pendingItems, source)
      options.message.success(`Vectorized in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
    } catch (error: any) {
      options.removePendingGeneratedImages(pendingItems)
      options.message.error(sanitizeApiErrorMessage(error?.msg || error?.message, 'Failed to vectorize image'))
    } finally {
      options.generating.value = false
    }
  }

  async function runCanvasAction(action: CanvasActionKey, item: CanvasImageItem) {
    if (options.generating.value) return
    options.selectedIds.value = [item.id]
    options.activeImageActionId.value = null
    options.showStyleStrengthControl.value = false
    if (action === 'remove-bg') {
      await handleRemoveBgGenerate(item)
      return
    }
    if (action === 'style-innovate') {
      await handleStyleInnovateGenerate(item)
      return
    }
    if (action === 'to-vector') {
      await handleToVectorGenerate(item)
      return
    }
    await handleSketchToDesignGenerate(item)
  }

  async function handleGenerate() {
    if (options.generating.value) return
    if (!options.hasDirectPrompt.value) return
    await handlePictureGenerate()
  }

  async function handleExport() {
    if (options.selectedIds.value.length === 0) return

    const exportableItems = options.messages.value.filter(
      m => options.selectedIds.value.includes(m.id) && m.type === 'image' && !!m.url,
    )

    if (exportableItems.length === 0) {
      options.message.warning('No exportable images selected')
      return
    }

    const timestamp = Date.now()
    let failed = 0

    if (exportLoading) {
      exportLoading.destroy()
      exportLoading = null
    }

    const loading = options.message.loading(`Exporting ${exportableItems.length} images...`, {
      duration: 1500,
      keepAliveOnHover: false,
    })
    exportLoading = loading

    try {
      for (let i = 0; i < exportableItems.length; i++) {
        const item = exportableItems[i]
        const fileName = buildExportFileName(item.url, i, exportableItems.length, timestamp)

        try {
          const link = document.createElement('a')
          link.href = item.url
          link.download = fileName
          link.rel = 'noopener'
          link.style.display = 'none'
          document.body.appendChild(link)
          link.click()
          document.body.removeChild(link)
        } catch (error) {
          failed += 1
          console.error('Export failed:', error)
        }
      }
    } catch (err) {
      failed += 1
      console.error('Export failed:', err)
    } finally {
      loading.destroy()
      if (exportLoading === loading) exportLoading = null
    }

    if (failed === 0) {
      options.message.success(`Export queued: ${exportableItems.length} image${exportableItems.length > 1 ? 's' : ''}`)
    } else {
      const success = exportableItems.length - failed
      options.message.warning(`Export queued ${success}/${exportableItems.length}, failed ${failed}`)
    }
  }

  return {
    runCanvasAction,
    handleGenerate,
    handlePictureGenerate,
    handleRemoveBgGenerate,
    handleSketchToDesignGenerate,
    handleStyleInnovateGenerate,
    handleToVectorGenerate,
    handleExport,
  }
}
