/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src/features/canvas')

function readSource(relativePath: string) {
  return readFileSync(resolve(sourceRoot, relativePath), 'utf8')
}

const nodeMotionSources = [
  'components/nodes/workflows/FusionStage.vue',
  'components/nodes/workflows/ImageNodeStage.vue',
  'image-tools/components/ChromeFrame.vue',
  'components/nodes/primitives/NodeAspectRatioPicker.vue',
  'components/nodes/primitives/NodePromptComposer.vue',
  'components/nodes/primitives/NodeStageFrame.vue',
  'components/nodes/primitives/NodeToolIconButton.vue',
  'components/nodes/styles/node-input-thumbnails.css',
  'components/nodes/styles/node-shared.css',
  'components/nodes/styles/node-tool-buttons.css',
  'components/styles/canvas-tool-tooltip.css',
].map(readSource)

const editorMotionSources = [
  'components/styles/canvas-range-input.css',
  'components/styles/editing-controls.css',
  'retouch/components/RetouchLayerActions.vue',
  'retouch/components/RetouchLayersPanel.vue',
  'inpaint/components/ImageInpaintOverlay.vue',
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
    const fusionStage = readSource('components/nodes/workflows/FusionStage.vue')

    expect(runningBorder).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.workflow-node\.running::before[\s\S]*animation: none;/,
    )
    expect(fusionStage).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.fusion-node-ray[\s\S]*animation: none;/,
    )
  })

  it('keeps precision cursors free from transform animation', () => {
    const maskOverlay = readSource('inpaint/components/ImageInpaintOverlay.vue')

    expect(maskOverlay).not.toMatch(/transition:\s*transform/)
  })
})
