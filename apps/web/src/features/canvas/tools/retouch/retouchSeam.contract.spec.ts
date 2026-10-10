import { describe, expect, it } from 'vitest'
import {
  cancelImageRetouchFinalRender,
  createRetouchApplyCommands,
  createRetouchChromeSource,
  hasRenderableRetouchLayerStack,
  renderImageRetouchFile,
  renderImageRetouchRecipeFile,
  renderRetouchLayerStackFile,
} from './index'
import { createRetouchApplyCommands as directCreateRetouchApplyCommands } from './session/applyRetouch'
import { createRetouchChromeSource as directCreateRetouchChromeSource } from './session/chromeSource'
import {
  cancelImageRetouchFinalRender as directCancelImageRetouchFinalRender,
  hasRenderableRetouchLayerStack as directHasRenderableRetouchLayerStack,
  renderImageRetouchFile as directRenderImageRetouchFile,
  renderImageRetouchRecipeFile as directRenderImageRetouchRecipeFile,
  renderRetouchLayerStackFile as directRenderRetouchLayerStackFile,
} from './session/stage/runtime/finalRender'

/**
 * Retouch public seam: image-tools reaches the apply pipeline, the chrome
 * source, and the export pipeline only through this barrel. These identity
 * assertions fail on a barrel drift instead of silently forking behavior.
 */
describe('retouch public seam', () => {
  it('exposes the apply pipeline through the barrel', () => {
    expect(createRetouchApplyCommands).toBe(directCreateRetouchApplyCommands)
  })

  it('exposes the chrome source through the barrel', () => {
    expect(createRetouchChromeSource).toBe(directCreateRetouchChromeSource)
  })

  it('exposes the runtime export pipeline through the barrel', () => {
    expect(renderImageRetouchFile).toBe(directRenderImageRetouchFile)
    expect(renderImageRetouchRecipeFile).toBe(directRenderImageRetouchRecipeFile)
    expect(renderRetouchLayerStackFile).toBe(directRenderRetouchLayerStackFile)
    expect(hasRenderableRetouchLayerStack).toBe(directHasRenderableRetouchLayerStack)
    expect(cancelImageRetouchFinalRender).toBe(directCancelImageRetouchFinalRender)
  })
})
