import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = resolve(import.meta.dirname, '..')

describe('Image Tool stage presentation contract', () => {
  it('keeps tool overlays and image-layer visibility outside the neutral media stage', () => {
    const stage = readFileSync(
      resolve(canvasRoot, '../ui/nodes/workflows/ImageNodeStage.vue'),
      'utf8',
    )
    const maskOverlay = readFileSync(
      resolve(canvasRoot, './inpaint/ui/ImageInpaintOverlay.vue'),
      'utf8',
    )

    expect(stage).not.toContain('ImageInpaintOverlay')
    expect(stage).not.toContain('RetouchRasterOverlay')
    expect(stage).toContain('imageToolStageBinding?.active')
    expect(stage).not.toContain('imageToolStageBinding?.mode')
    expect(stage).not.toContain('imageToolStageBinding?.renderedPreview')
    expect(maskOverlay).not.toContain('.image-node-stage:has(')
  })
})
