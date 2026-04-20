import type { ComputedRef, Ref } from 'vue'
import type { CanvasImageItem } from '../types/canvasNodes'

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
  request: {
    prompt: string
    feature?: string
    card_ids?: number[]
    params?: Record<string, unknown>
  }
  source?: CanvasImageItem
  successMessage: string
  failureMessage: string
  onSubmitted?: () => void
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
  defaultSegType: SegType
  message: MessageApiLike
  submitTaskAndPoll: (options: SubmitTaskAndPollOptions) => Promise<string | null>
  appendGeneratedImages: (urls: string[], source?: CanvasImageItem) => void
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

export function useCanvasDirectActions(options: UseCanvasDirectActionsOptions) {
  let exportLoading: MessageLoadingHandle | null = null

  async function handlePictureGenerate() {
    const promptText = resolveDirectPromptText(options.userPrompt.value)
    if (!promptText) return
    options.generating.value = true
    await options.submitTaskAndPoll({
      request: {
        prompt: promptText,
        feature: 'pattern-gen',
        params: {
          feature: 'pattern-gen',
          ratio: options.selectedRatio.value,
          quality: options.selectedQuality.value,
        },
      },
      successMessage: '任务已提交',
      failureMessage: '提交失败',
      onSubmitted: () => {
        options.userPrompt.value = ''
        options.showSettings.value = false
      },
    })
  }

  async function handleRemoveBgGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const startTime = Date.now()
    try {
      const segmentRes: any = await options.segmentImage({
        image_url: source.url,
        seg_type: options.defaultSegType,
      })
      if (segmentRes.code !== 0 || !segmentRes.data?.result_url) {
        options.message.error(segmentRes.msg || 'Segmentation failed')
        return
      }

      const saveRes: any = await options.saveSegmentResult({
        result_url: segmentRes.data.result_url,
        seg_type: options.defaultSegType,
        feature: 'remove-bg',
      })
      if (saveRes.code !== 0 || !saveRes.data?.image_url) {
        options.message.error(saveRes.msg || 'Save result failed')
        return
      }

      options.appendGeneratedImages([saveRes.data.image_url], source)
      options.message.success(`Background removed in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
    } catch (error: any) {
      options.message.error(error?.msg || error?.message || 'Failed to remove background')
    } finally {
      options.generating.value = false
    }
  }

  async function handleSketchToDesignGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const desc = resolveDirectPromptText(options.userPrompt.value, 'Sketch to design')
    await options.submitTaskAndPoll({
      request: {
        prompt: `[sketch-to-design] ${desc}`,
        params: {
          image_urls: [source.url],
          feature: 'sketch-to-design',
        },
      },
      source,
      successMessage: 'Task submitted',
      failureMessage: 'Failed to submit task',
      onSubmitted: () => {
        options.userPrompt.value = ''
      },
    })
  }

  async function handleStyleInnovateGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const desc = resolveDirectPromptText(options.userPrompt.value, 'Style innovate')
    const strength = options.styleInnovateStrength.value
    await options.submitTaskAndPoll({
      request: {
        prompt: `[style-innovate] 改变幅度：${strength}/10，${desc}`,
        params: {
          image_urls: [source.url],
          feature: 'style-innovate',
          variation_strength: strength,
        },
      },
      source,
      successMessage: 'Task submitted',
      failureMessage: 'Failed to submit task',
      onSubmitted: () => {
        options.userPrompt.value = ''
      },
    })
  }

  async function handleToVectorGenerate(source: CanvasImageItem) {
    options.generating.value = true
    const startTime = Date.now()
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
        options.message.error(res.msg || 'Vectorize failed')
        return
      }

      options.appendGeneratedImages([res.data.svg_url], source)
      options.message.success(`Vectorized in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
    } catch (error: any) {
      options.message.error(error?.msg || error?.message || 'Failed to vectorize image')
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
