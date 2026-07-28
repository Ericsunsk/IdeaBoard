import { describe, expect, it, vi } from 'vitest'
import {
  createFusionNodePresentationIntentHandler,
  createFusionNodePresentationResolver,
} from './fusionNodePresentation'

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

  it('translates input, parameter, context, deletion, and generation intents into Canvas commands', () => {
    const onParameterChanged = vi.fn()
    const onDeleteRequested = vi.fn()
    const onMediaInputDrop = vi.fn()
    const onMediaInputRemove = vi.fn()
    const onContextMenuRequested = vi.fn()
    const onGenerationRequested = vi.fn()
    const handleIntent = createFusionNodePresentationIntentHandler({
      onParameterChanged,
      onDeleteRequested,
      onMediaInputDrop,
      onMediaInputRemove,
      onContextMenuRequested,
      onGenerationRequested,
    })
    const dropEvent = new DragEvent('drop')
    const contextEvent = new MouseEvent('contextmenu')

    handleIntent({
      type: 'parameter-changed',
      nodeId: 'fusion-1',
      patch: { key: 'blendPoint', value: { x: 0.25, y: 0.75 } },
    })
    handleIntent({ type: 'media-input-dropped', nodeId: 'fusion-1', event: dropEvent })
    handleIntent({ type: 'media-input-remove-requested', nodeId: 'fusion-1', imageId: 'image-1' })
    handleIntent({ type: 'context-menu-requested', nodeId: 'fusion-1', event: contextEvent })
    handleIntent({ type: 'delete-requested', nodeId: 'fusion-1' })
    handleIntent({
      type: 'generation-requested',
      nodeId: 'fusion-1',
      prompt: '融合两张图片',
      outputCount: 2,
      aspectRatio: '1:1',
    })

    expect(onParameterChanged).toHaveBeenCalledWith('fusion-1', {
      key: 'blendPoint',
      value: { x: 0.25, y: 0.75 },
    })
    expect(onMediaInputDrop).toHaveBeenCalledWith('fusion-1', dropEvent)
    expect(onMediaInputRemove).toHaveBeenCalledWith('fusion-1', 'image-1')
    expect(onContextMenuRequested).toHaveBeenCalledWith('fusion-1', contextEvent)
    expect(onDeleteRequested).toHaveBeenCalledWith('fusion-1')
    expect(onGenerationRequested).toHaveBeenCalledWith('fusion-1', '融合两张图片', 2, '1:1')
  })
})
