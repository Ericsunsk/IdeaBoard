/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src/features/canvas')

function readSource(relativePath: string) {
  return readFileSync(resolve(sourceRoot, relativePath), 'utf8')
}

const nodeMotionSources = [
  'components/nodes/FusionStage.vue',
  'components/nodes/ImageNodeStage.vue',
  'components/nodes/ImageToolChromeFrame.vue',
  'components/nodes/NodeAspectRatioPicker.vue',
  'components/nodes/NodePromptComposer.vue',
  'components/nodes/NodeStageFrame.vue',
  'components/nodes/NodeToolIconButton.vue',
  'components/nodes/styles/node-input-thumbnails.css',
  'components/nodes/styles/node-shared.css',
  'components/nodes/styles/node-tool-buttons.css',
].map(readSource)

const editorMotionSources = [
  'components/styles/canvas-range-input.css',
  'components/styles/editing-controls.css',
  'image-editor/components/ImageEditorLayerActions.vue',
  'image-editor/components/ImageEditorLayersPanel.vue',
  'image-editor/components/ImageEditorRasterOverlay.vue',
  'mask-editor/components/ImageMaskEditorOverlay.vue',
].map(readSource)

describe('Canvas Node and editor Motion contract', () => {
  it('maps Node and editor presentation to semantic Motion Policy tokens', () => {
    const presentationStyles = [...nodeMotionSources, ...editorMotionSources].join('\n')

    expect(presentationStyles).toContain('var(--motion-control-duration)')
    expect(presentationStyles).toContain('var(--motion-enter-duration)')
    expect(presentationStyles).toContain('var(--motion-exit-duration)')
    expect(presentationStyles).toContain('var(--motion-feedback-duration)')
    expect(presentationStyles).not.toContain('var(--canvas-motion-')
  })

  it('stops continuous running and apply motion when reduced motion is requested', () => {
    const runningBorder = readSource('components/nodes/styles/node-shared.css')
    const imageStage = readSource('components/nodes/ImageNodeStage.vue')
    const fusionStage = readSource('components/nodes/FusionStage.vue')
    const layerActions = readSource('image-editor/components/ImageEditorLayerActions.vue')

    expect(runningBorder).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.workflow-node\.running::before[\s\S]*animation: none;/,
    )
    expect(imageStage).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.image-node-stage-loader-logo[\s\S]*animation: none;/,
    )
    expect(fusionStage).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.fusion-node-ray[\s\S]*animation: none;/,
    )
    expect(layerActions).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.image-editor-layer-actions__progress-bar[\s\S]*animation: none;/,
    )
  })

  it('keeps precision cursors free from transform animation', () => {
    const rasterOverlay = readSource('image-editor/components/ImageEditorRasterOverlay.vue')
    const maskOverlay = readSource('mask-editor/components/ImageMaskEditorOverlay.vue')

    expect(rasterOverlay).not.toMatch(/transition:\s*transform/)
    expect(maskOverlay).not.toMatch(/transition:\s*transform/)
  })
})
