import { describe, expect, it } from 'vitest'
import { createFusionNodePresentationResolver } from './fusionNodePresentation'

describe('Fusion Node presentation seam', () => {
  it('keeps the Fusion presentation binding stable until visibility changes', () => {
    let isPromptComposerVisible = false
    const resolver = createFusionNodePresentationResolver({
      promptComposerVisibleFor: () => isPromptComposerVisible,
    })

    const initial = resolver.forNode('fusion-1')
    expect(resolver.forNode('fusion-1')).toBe(initial)
    expect(initial).toEqual({ isPromptComposerVisible: false })

    isPromptComposerVisible = true
    expect(resolver.forNode('fusion-1')).toEqual({ isPromptComposerVisible: true })
    expect(resolver.forNode('fusion-1')).not.toBe(initial)
  })
})
