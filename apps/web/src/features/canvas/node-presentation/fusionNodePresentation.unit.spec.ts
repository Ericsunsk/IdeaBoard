import { describe, expect, it } from 'vitest'
import { createFusionNodePresentationResolver } from './fusionNodePresentation'

describe('Fusion Node presentation seam', () => {
  it('keeps the Fusion presentation binding stable until visibility changes', () => {
    let promptComposerVisible = false
    const resolver = createFusionNodePresentationResolver({
      promptComposerVisibleFor: () => promptComposerVisible,
    })

    const initial = resolver.forNode('fusion-1')
    expect(resolver.forNode('fusion-1')).toBe(initial)
    expect(initial).toEqual({ promptComposerVisible: false })

    promptComposerVisible = true
    expect(resolver.forNode('fusion-1')).toEqual({ promptComposerVisible: true })
    expect(resolver.forNode('fusion-1')).not.toBe(initial)
  })
})
