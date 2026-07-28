import { describe, expect, it, vi } from 'vitest'
import type { ImageActionBinding, ImageToolStageBinding } from '../image-tools'
import {
  createImageNodePresentationIntentHandler,
  createImageNodePresentationResolver,
} from './imageNodePresentation'

describe('Image Node presentation seam', () => {
  it('reuses a node binding until its action or stage binding changes', () => {
    let actionBinding = { actionMenuVisible: true } as ImageActionBinding
    let stageBinding = { mode: 'adjust' } as ImageToolStageBinding
    const resolver = createImageNodePresentationResolver({
      actionBindingFor: () => actionBinding,
      stageBindingFor: () => stageBinding,
    })

    const initial = resolver.forNode('image-1')
    expect(resolver.forNode('image-1')).toBe(initial)
    expect(Object.keys(initial)).toEqual(['actionBinding', 'stageBinding'])

    actionBinding = { actionMenuVisible: false } as ImageActionBinding
    const afterActionChange = resolver.forNode('image-1')
    expect(afterActionChange).not.toBe(initial)
    expect(afterActionChange.stageBinding).toBe(stageBinding)

    stageBinding = { mode: 'inpaint' } as ImageToolStageBinding
    expect(resolver.forNode('image-1')).not.toBe(afterActionChange)
  })

  it('translates media and preview intents into Canvas commands', () => {
    const onFileOpenRequested = vi.fn()
    const onPreviewRequested = vi.fn()
    const onAppendToCanvasRequested = vi.fn()
    const onResetRequested = vi.fn()
    const handleIntent = createImageNodePresentationIntentHandler({
      onParameterChanged: vi.fn(),
      onDeleteRequested: vi.fn(),
      onStageDrop: vi.fn(),
      onFileOpenRequested,
      onPreviewRequested,
      onAppendToCanvasRequested,
      onResetRequested,
      onContextMenuRequested: vi.fn(),
      onPromptGenerationRequested: vi.fn(),
      onCanvasExtendSubmitted: vi.fn(),
      onImageActionRequested: vi.fn(),
      onCanvasExtendHandlePointerDown: vi.fn(),
      onCanvasExtendHandlePointerMove: vi.fn(),
      onCanvasExtendHandlePointerUp: vi.fn(),
      onCanvasExtendFramePointerDown: vi.fn(),
    })

    handleIntent({ type: 'file-open-requested', nodeId: 'image-1' })
    handleIntent({ type: 'preview-requested', nodeId: 'image-1' })
    handleIntent({ type: 'append-to-canvas-requested', nodeId: 'image-1' })
    handleIntent({ type: 'reset-requested', nodeId: 'image-1' })

    expect(onFileOpenRequested).toHaveBeenCalledWith('image-1')
    expect(onPreviewRequested).toHaveBeenCalledWith('image-1')
    expect(onAppendToCanvasRequested).toHaveBeenCalledWith('image-1')
    expect(onResetRequested).toHaveBeenCalledWith('image-1')
  })

  it('keeps action, Canvas Extend, and context intents at the presentation boundary', () => {
    const onImageActionRequested = vi.fn()
    const onCanvasExtendSubmitted = vi.fn()
    const onContextMenuRequested = vi.fn()
    const handleIntent = createImageNodePresentationIntentHandler({
      onParameterChanged: vi.fn(),
      onDeleteRequested: vi.fn(),
      onStageDrop: vi.fn(),
      onFileOpenRequested: vi.fn(),
      onPreviewRequested: vi.fn(),
      onAppendToCanvasRequested: vi.fn(),
      onResetRequested: vi.fn(),
      onContextMenuRequested,
      onPromptGenerationRequested: vi.fn(),
      onCanvasExtendSubmitted,
      onImageActionRequested,
      onCanvasExtendHandlePointerDown: vi.fn(),
      onCanvasExtendHandlePointerMove: vi.fn(),
      onCanvasExtendHandlePointerUp: vi.fn(),
      onCanvasExtendFramePointerDown: vi.fn(),
    })
    const contextEvent = new MouseEvent('contextmenu')

    handleIntent({ type: 'action-requested', nodeId: 'image-1', action: 'remove-bg' })
    handleIntent({
      type: 'canvas-extend-submitted',
      nodeId: 'image-1',
      prompt: '向右扩图',
      outputCount: 2,
    })
    handleIntent({ type: 'context-menu-requested', nodeId: 'image-1', event: contextEvent })

    expect(onImageActionRequested).toHaveBeenCalledWith('image-1', 'remove-bg')
    expect(onCanvasExtendSubmitted).toHaveBeenCalledWith('image-1', '向右扩图', 2)
    expect(onContextMenuRequested).toHaveBeenCalledWith('image-1', contextEvent)
  })
})
