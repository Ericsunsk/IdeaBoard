import { normalizePromptAspectRatio } from '../../canvas-board/domain'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { MediaInputImage } from '../graph-editing/useFlowMediaInputImages'

export type GenerationIntentOptions = {
  cancelFlowOutpaintMode: () => void
  cancelFlowInpaintMode: () => void
  getMediaInputImages: (nodeId: string) => MediaInputImage[]
  runGeneration: (
    nodeId: string,
    referenceImages: readonly { url: string; ratio: number; mediaAssetId?: number }[],
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
        ...(item.mediaAssetId ? { mediaAssetId: item.mediaAssetId } : {}),
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
