import type { ImageEditorRenderedPreview } from '../types'
import type { ImageProcessingRuntime } from '../runtime/types'
import { getImageAdjustmentProcessingRuntime } from '../runtime/imageAdjustmentProcessingRuntime'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import { measureCanvasPerfAsync } from '../../utils/canvasPerfProbe'

const PREVIEW_MAX_SIDE = 960
const INLINE_PREVIEW_TASK_ID = 'image-adjustment:inline-preview'

type ImageAdjustmentPreviewSource = {
  sourceUrl: string
  imageData: ImageData
  width: number
  height: number
}

type ImageAdjustmentInlinePreviewRendererOptions = {
  onPreview: (preview: ImageEditorRenderedPreview | null) => void
  onSource?: (source: ImageAdjustmentPreviewSource | null) => void
  onError?: (error: unknown) => void
  runtime?: ImageProcessingRuntime
}

export type ImageAdjustmentInlinePreviewRenderer = {
  updateSource: (sourceUrl: string | null) => void
  updatePlan: (plan: ImageAdjustmentPlan) => void
  dispose: () => void
}

export function createImageAdjustmentInlinePreviewRenderer(
  options: ImageAdjustmentInlinePreviewRendererOptions,
): ImageAdjustmentInlinePreviewRenderer {
  let currentPlan: ImageAdjustmentPlan = []
  let currentSourceUrl: string | null = null
  let sourceRequestToken = 0
  let previewFrameId: number | null = null
  const runtime = options.runtime ?? getImageAdjustmentProcessingRuntime()

  return {
    updateSource(sourceUrl) {
      if (sourceUrl === currentSourceUrl) return
      currentSourceUrl = sourceUrl
      sourceRequestToken += 1
      options.onPreview(null)
      options.onSource?.(null)
      cancelPreviewFrame()
      if (!sourceUrl) return

      scheduleRenderedPreview()
    },
    updatePlan(plan) {
      currentPlan = plan
      scheduleRenderedPreview()
    },
    dispose() {
      sourceRequestToken += 1
      currentSourceUrl = null
      currentPlan = []
      options.onPreview(null)
      options.onSource?.(null)
      cancelPreviewFrame()
      runtime.cancel(INLINE_PREVIEW_TASK_ID)
    },
  }

  function scheduleRenderedPreview() {
    cancelPreviewFrame()
    if (!currentSourceUrl) return

    let didRunSynchronously = false
    const frameId = window.requestAnimationFrame(() => {
      didRunSynchronously = true
      previewFrameId = null
      renderLatestPreview(currentSourceUrl, currentPlan, sourceRequestToken)
    })
    previewFrameId = didRunSynchronously ? null : frameId
  }

  function cancelPreviewFrame() {
    if (previewFrameId == null) return
    window.cancelAnimationFrame(previewFrameId)
    previewFrameId = null
  }

  function renderLatestPreview(
    sourceUrl: string | null,
    plan: ImageAdjustmentPlan,
    requestToken: number,
  ) {
    if (!sourceUrl) {
      options.onPreview(null)
      return
    }

    void measureCanvasPerfAsync(
      'image.preview.render',
      () => ({
        adjustments: plan.length,
        latestOnly: true,
        stale: requestToken !== sourceRequestToken,
      }),
      () =>
        runtime.renderPreview({
          source: { url: sourceUrl },
          plan,
          taskId: INLINE_PREVIEW_TASK_ID,
          latestOnly: true,
          sizeStrategy: { maxSide: PREVIEW_MAX_SIDE },
        }),
      50,
    )
      .then((result) => {
        if (requestToken !== sourceRequestToken) return
        if (result.status !== 'completed') {
          if (result.status === 'failed') options.onError?.(result.error)
          return
        }
        const source = {
          sourceUrl,
          imageData: result.output.sourceImageData,
          width: result.output.width,
          height: result.output.height,
        }
        options.onSource?.(source)
        options.onPreview(
          plan.length > 0
            ? {
                type: 'rendered-image',
                imageData: result.output.previewImageData,
                width: result.output.width,
                height: result.output.height,
              }
            : null,
        )
      })
      .catch((error: unknown) => {
        if (requestToken !== sourceRequestToken) return
        options.onError?.(error)
      })
  }
}
