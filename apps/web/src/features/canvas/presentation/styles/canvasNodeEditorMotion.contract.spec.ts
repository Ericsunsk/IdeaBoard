/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src/features/canvas')

function readSource(relativePath: string) {
  return readFileSync(resolve(sourceRoot, relativePath), 'utf8')
}

const nodeMotionSources = [
  'presentation/nodes/workflows/FusionStage.vue',
  'presentation/nodes/workflows/ImageNodeStage.vue',
  'tools/image-tools/components/ChromeFrame.vue',
  'presentation/nodes/primitives/NodeAspectRatioPicker.vue',
  'presentation/nodes/primitives/NodePromptComposer.vue',
  'presentation/nodes/primitives/NodeStageFrame.vue',
  'presentation/nodes/primitives/NodeToolIconButton.vue',
  'presentation/nodes/styles/node-input-thumbnails.css',
  'presentation/nodes/styles/node-shared.css',
  'presentation/nodes/styles/node-tool-buttons.css',
  'presentation/styles/canvas-tool-tooltip.css',
].map(readSource)

const editorMotionSources = [
  'presentation/styles/canvas-range-input.css',
  'presentation/styles/editing-controls.css',
  'tools/retouch/components/LayerActions.vue',
  'tools/retouch/components/LayersPanel.vue',
  'tools/inpaint/components/ImageInpaintOverlay.vue',
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
    const runningBorder = readSource('presentation/nodes/styles/node-shared.css')
    const fusionStage = readSource('presentation/nodes/workflows/FusionStage.vue')

    expect(runningBorder).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.workflow-node\.running::before[\s\S]*animation: none;/,
    )
    expect(fusionStage).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.fusion-node-ray[\s\S]*animation: none;/,
    )
  })

  it('keeps precision cursors free from transform animation', () => {
    const maskOverlay = readSource('tools/inpaint/components/ImageInpaintOverlay.vue')

    expect(maskOverlay).not.toMatch(/transition:\s*transform/)
  })
})
