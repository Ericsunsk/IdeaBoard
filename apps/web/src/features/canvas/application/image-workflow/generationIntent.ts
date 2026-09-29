import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../../board/ports/canvas'
import type { MediaInputImage } from '../editing/useFlowMediaInputImages.ts'
import type { MediaRef } from '../../../../shared/media/mediaRef'

export type GenerationIntentOptions = {
  cancelFlowOutpaintMode: () => void
  cancelFlowInpaintMode: () => void
  getMediaInputImages: (nodeId: string) => MediaInputImage[]
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
    void options.runGeneration(
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
  }

  return {
    onFlowPromptGenerate,
  }
}
