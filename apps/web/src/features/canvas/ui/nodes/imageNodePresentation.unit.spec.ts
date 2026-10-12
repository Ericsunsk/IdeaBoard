import { describe, expect, it } from 'vitest'
import type { ImageActionBinding, ImageToolStageBinding } from '../../tools/image-tools'
import { createImageNodePresentationResolver } from './imageNodePresentation'

describe('Image Node presentation seam', () => {
  it('reuses a node binding until its action or stage binding changes', () => {
    let actionBinding = { isActionMenuVisible: true } as ImageActionBinding
    let stageBinding = { mode: 'adjust' } as ImageToolStageBinding
    const resolver = createImageNodePresentationResolver({
      actionBindingFor: () => actionBinding,
      stageBindingFor: () => stageBinding,
    })

    const initial = resolver.forNode('image-1')
    expect(resolver.forNode('image-1')).toBe(initial)
    expect(Object.keys(initial)).toEqual(['actionBinding', 'stageBinding'])

    actionBinding = { isActionMenuVisible: false } as ImageActionBinding
    const afterActionChange = resolver.forNode('image-1')
    expect(afterActionChange).not.toBe(initial)
    expect(afterActionChange.stageBinding).toBe(stageBinding)

    stageBinding = { mode: 'inpaint' } as ImageToolStageBinding
    expect(resolver.forNode('image-1')).not.toBe(afterActionChange)
  })
})
