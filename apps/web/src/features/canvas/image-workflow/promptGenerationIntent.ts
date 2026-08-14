import { normalizePromptAspectRatio } from '../../canvas-board/domain'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { MediaInputImage } from '../composables/useFlowMediaInputImages'

export type PromptGenerationIntentOptions = {
  cancelFlowExpandMode: () => void
  cancelFlowMaskEditorMode: () => void
  getMediaInputImages: (nodeId: string) => MediaInputImage[]
  runPromptGeneration: (
    nodeId: string,
    referenceImages: readonly { url: string; ratio: number }[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void> | void
}

export function createPromptGenerationIntent(options: PromptGenerationIntentOptions) {
  function onFlowPromptGenerate(
    promptNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const inputImages = options.getMediaInputImages(promptNodeId)
    void options.runPromptGeneration(
      promptNodeId,
      inputImages.map((item) => ({ url: item.url, ratio: item.ratio })),
      prompt,
      outputCount,
      normalizedAspectRatio,
    )
  }

  return {
    onFlowPromptGenerate,
  }
}
