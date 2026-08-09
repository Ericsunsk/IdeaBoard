import { normalizePromptAspectRatio } from '../../canvas-board/domain'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { MediaInputImage } from '../composables/useFlowMediaInputImages'

export type PromptGenerationIntentOptions = {
  cancelFlowExpandMode: () => void
  cancelFlowMaskEditorMode: () => void
  getMediaInputImages: (nodeId: string) => MediaInputImage[]
  resolvePromptInputImageNodeIds: (nodeId: string) => string[]
  runTextPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void> | void
  runImagePromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    options?: { outputAnchorNodeId?: string },
  ) => Promise<void> | void
  runImagePromptGenerationFromSource: (
    sourceImage: { url: string; ratio: number },
    sourceKey: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    options?: { outputAnchorNodeId?: string },
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
    if (inputImages.length === 0) {
      void options.runTextPromptGeneration(promptNodeId, prompt, outputCount, normalizedAspectRatio)
      return
    }
    const linkedSourceNodeIds = new Set(options.resolvePromptInputImageNodeIds(promptNodeId))
    inputImages.forEach((item) => {
      if (item.sourceType === 'node' && linkedSourceNodeIds.has(item.sourceId)) {
        void options.runImagePromptGeneration(
          item.sourceId,
          prompt,
          outputCount,
          normalizedAspectRatio,
          { outputAnchorNodeId: promptNodeId },
        )
        return
      }
      void options.runImagePromptGenerationFromSource(
        {
          url: item.url,
          ratio: item.ratio,
        },
        item.id,
        prompt,
        outputCount,
        normalizedAspectRatio,
        { outputAnchorNodeId: promptNodeId },
      )
    })
  }

  return {
    onFlowPromptGenerate,
  }
}
