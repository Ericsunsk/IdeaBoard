import { describe, expect, it, vi } from 'vitest'
import { createCanvasNodePresentationIntentHandler } from './canvasNodePresentation'

describe('Canvas Node presentation contract', () => {
  it('routes each node kind through the single public intent dispatcher', () => {
    const prompt = vi.fn()
    const image = vi.fn()
    const fusion = vi.fn()
    const handleIntent = createCanvasNodePresentationIntentHandler({ prompt, image, fusion })

    const promptIntent = {
      nodeKind: 'prompt' as const,
      type: 'text-edited' as const,
      nodeId: 'prompt-1',
      text: '新的提示词',
    }
    const imageIntent = {
      nodeKind: 'image' as const,
      type: 'reset-requested' as const,
      nodeId: 'image-1',
    }
    const fusionIntent = {
      nodeKind: 'fusion' as const,
      type: 'delete-requested' as const,
      nodeId: 'fusion-1',
    }

    handleIntent(promptIntent)
    handleIntent(imageIntent)
    handleIntent(fusionIntent)

    expect(prompt).toHaveBeenCalledWith(promptIntent)
    expect(image).toHaveBeenCalledWith(imageIntent)
    expect(fusion).toHaveBeenCalledWith(fusionIntent)
  })
})
