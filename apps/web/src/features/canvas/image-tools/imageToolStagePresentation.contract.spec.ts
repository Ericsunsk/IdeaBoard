import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = resolve(import.meta.dirname, '..')

describe('Image Tool stage presentation contract', () => {
  it('keeps tool overlays and image-layer visibility outside the neutral media stage', () => {
    const stage = readFileSync(resolve(canvasRoot, 'components/nodes/ImageNodeStage.vue'), 'utf8')
    const maskOverlay = readFileSync(
      resolve(canvasRoot, 'mask-editor/components/ImageMaskEditorOverlay.vue'),
      'utf8',
    )

    expect(stage).not.toContain('ImageMaskEditorOverlay')
    expect(stage).not.toContain('ImageEditorRasterOverlay')
    expect(stage).toContain('imageToolStageBinding?.active')
    expect(stage).not.toContain('imageToolStageBinding?.mode')
    expect(stage).not.toContain('imageToolStageBinding?.renderedPreview')
    expect(maskOverlay).not.toContain('.image-node-stage:has(')
  })
})
