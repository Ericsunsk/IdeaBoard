import { describe, expect, it } from 'vitest'
import { useImageEditorSession } from './useImageEditorSession'

describe('useImageEditorSession', () => {
  it('keeps canvas tools independent from adjustment layer selection', () => {
    const session = useImageEditorSession()

    session.selectImageEditorTool('brush')

    expect(session.activeImageEditorTool.value).toBe('brush')
    expect(session.selectedImageEditorAdjustmentLayer.value?.kind).toBe('hsl')

    session.selectImageAdjustmentLayer('threshold')

    expect(session.activeImageEditorTool.value).toBe('brush')
    expect(session.selectedImageEditorAdjustmentLayer.value?.kind).toBe('threshold')
    expect(session.imageEditorLayers.value.some((layer) => layer.id === 'adjustment:threshold'))
      .toBe(true)
  })

  it('selects source layers without changing the active tool', () => {
    const session = useImageEditorSession()

    session.selectImageEditorTool('eraser')
    session.selectImageEditorLayer('source')

    expect(session.activeImageEditorTool.value).toBe('eraser')
    expect(session.selectedImageEditorAdjustmentLayer.value).toBeNull()
  })
})
