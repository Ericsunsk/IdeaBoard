import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../../board/ports/canvas'
import { i18n } from '../../../../core/i18n'
import type { MediaInputImage } from '../graph-editing'
import type { MediaRef } from '../../../../core/media/mediaRef'

export type GenerationIntentOptions = {
  cancelFlowOutpaintMode: () => void
  cancelFlowInpaintMode: () => void
  getMediaInputImages: (nodeId: string) => MediaInputImage[]
  onWarning: (message: string) => void
  runGeneration: (
    nodeId: string,
    referenceImages: readonly { url: string; ratio: number; mediaRef?: MediaRef }[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void> | void
}

export function createGenerationIntent(options: GenerationIntentOptions) {
  function onFlowPromptGenerate(
    promptNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    options.cancelFlowOutpaintMode()
    options.cancelFlowInpaintMode()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const inputImages = options.getMediaInputImages(promptNodeId)
    const result = options.runGeneration(
      promptNodeId,
      inputImages.map((item) => ({
        url: item.url,
        ratio: item.ratio,
        ...(item.mediaRef ? { mediaRef: item.mediaRef } : {}),
      })),
      prompt,
      outputCount,
      normalizedAspectRatio,
    )
    // runGeneration only rejects for non-planning failures (planning errors
    // already surfaced as warnings inside executePlannedAction): never let a
    // fire-and-forget submit die silently again.
    if (result && typeof (result as Promise<void>).catch === 'function') {
      void (result as Promise<void>).catch(() =>
        options.onWarning(i18n.global.t('canvas.execution.imageGenerationFailed')),
      )
    }
  }

  return {
    onFlowPromptGenerate,
  }
}
