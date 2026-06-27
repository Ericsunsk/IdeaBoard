import { describe, expect, it } from 'vitest'
import { useImageEditorSession } from './useImageEditorSession'
import { flattenVisibleImageEditorAdjustmentLayers } from '../utils/imageEditorLayerAdjustments'

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

  it('adds paint layers above the selected editable layer', () => {
    const session = useImageEditorSession()

    session.addImageEditorLayer()
    session.addImageEditorLayer()

    const [secondPaintLayer, firstPaintLayer, hslLayer, sourceLayer] =
      session.imageEditorLayers.value
    expect(secondPaintLayer?.type).toBe('paint')
    expect(secondPaintLayer?.name).toBe('图层 2')
    expect(secondPaintLayer?.type === 'paint' ? secondPaintLayer.content : null).toEqual({
      kind: 'empty',
    })
    expect(firstPaintLayer?.type).toBe('paint')
    expect(firstPaintLayer?.name).toBe('图层 1')
    expect(hslLayer?.type).toBe('adjustment')
    expect(sourceLayer?.type).toBe('source')
    expect(session.selectedImageEditorLayerId.value).toBe(secondPaintLayer?.id)
  })

  it('adds paint layers above the source when source is selected', () => {
    const session = useImageEditorSession()

    session.selectImageEditorLayer('source')
    session.addImageEditorLayer()

    const [hslLayer, paintLayer, sourceLayer] = session.imageEditorLayers.value
    expect(paintLayer?.type).toBe('paint')
    expect(paintLayer?.name).toBe('图层 1')
    expect(hslLayer?.type).toBe('adjustment')
    expect(sourceLayer?.type).toBe('source')
    expect(session.selectedImageEditorLayerId.value).toBe(paintLayer?.id)
  })

  it('copies editable layers with unique ids and names', () => {
    const session = useImageEditorSession()

    session.copyImageEditorLayer()

    const [copyLayer, originalLayer] = session.imageEditorLayers.value
    expect(copyLayer?.type).toBe('adjustment')
    expect(copyLayer?.id).not.toBe(originalLayer?.id)
    expect(copyLayer?.name).toBe('色相/饱和度 1 副本')
    expect(originalLayer?.type).toBe('adjustment')
    expect(session.selectedImageEditorLayerId.value).toBe(copyLayer?.id)
  })

  it('copies paint layers as independent empty raster layers', () => {
    const session = useImageEditorSession()

    session.addImageEditorLayer()
    session.copyImageEditorLayer()

    const [copyLayer, originalLayer] = session.imageEditorLayers.value
    expect(copyLayer?.type).toBe('paint')
    expect(copyLayer?.name).toBe('图层 1 副本')
    expect(copyLayer?.id).not.toBe(originalLayer?.id)
    expect(copyLayer?.type === 'paint' ? copyLayer.content : null).toEqual({ kind: 'empty' })
  })

  it('merges the selected editable layer down and preserves visible adjustments', () => {
    const session = useImageEditorSession()
    session.selectImageAdjustmentLayer('threshold')
    session.selectImageAdjustmentLayer('hsl')
    session.mergeImageEditorLayer()

    const mergedLayer = session.imageEditorLayers.value[0]
    expect(mergedLayer?.type).toBe('merged')
    expect(mergedLayer?.name).toBe('合并图层 1')
    expect(session.selectedImageEditorLayerId.value).toBe(mergedLayer?.id)
    expect(flattenVisibleImageEditorAdjustmentLayers(session.imageEditorLayers.value).map(
      (layer) => layer.kind,
    )).toEqual(['hsl', 'threshold'])
  })

  it('removes selected editable layers and selects the nearest editable fallback', () => {
    const session = useImageEditorSession()
    session.selectImageAdjustmentLayer('threshold')
    session.selectImageAdjustmentLayer('hsl')

    session.removeImageEditorLayer('adjustment:hsl')

    expect(session.imageEditorLayers.value.map((layer) => layer.id)).toEqual([
      'adjustment:threshold',
      'source',
    ])
    expect(session.selectedImageEditorLayerId.value).toBe('adjustment:threshold')
  })

  it('keeps source layers protected from copy, merge, and remove actions', () => {
    const session = useImageEditorSession()
    session.selectImageEditorLayer('source')

    session.copyImageEditorLayer()
    session.mergeImageEditorLayer()
    session.removeImageEditorLayer('source')

    expect(session.imageEditorLayers.value.map((layer) => layer.id)).toEqual([
      'adjustment:hsl',
      'source',
    ])
    expect(session.selectedImageEditorLayerId.value).toBe('source')
  })
})
