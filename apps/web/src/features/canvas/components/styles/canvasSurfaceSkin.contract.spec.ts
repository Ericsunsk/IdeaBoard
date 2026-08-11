/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src/features/canvas')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const canvasScreen = readSource('screen-presentation/CanvasScreen.vue')
const surfaceSkin = readSource('components/styles/canvas-surface-skin.css')
const chromeSkin = readSource('components/styles/canvas-chrome.css')
const nodeSkin = readSource('components/nodes/styles/node-shared.css')
const nodeToolSkin = readSource('components/nodes/styles/node-tool-buttons.css')
const nodeStageFrame = readSource('components/nodes/NodeStageFrame.vue')
const vueFlowAdapterSkin = readSource('components/styles/canvas-vue-flow-adapter-skin.css')
const canvasMiniMap = readSource('components/CanvasMiniMap.vue')
const imageEditorToolRail = readSource('image-editor/components/ImageEditorToolRail.vue')
const imageEditorLayerActions = readSource('image-editor/components/ImageEditorLayerActions.vue')
const imageEditorHistogram = readSource(
  'image-editor/components/styles/image-editor-histogram-plot.css',
)
const maskEditorChrome = readSource('mask-editor/components/ImageMaskEditorChrome.vue')

describe('Canvas surface skin contract', () => {
  it('mounts one feature-local surface owner instead of declaring the stable token table in Canvas Screen', () => {
    expect(canvasScreen).toContain('canvas-surface-skin')
    expect(canvasScreen).not.toMatch(/--canvas-[a-z0-9-]+\s*:/)
    expect(canvasScreen).not.toMatch(/--flow-[a-z0-9-]+\s*:/)
    expect(surfaceSkin).toContain('.canvas-surface-skin')
  })

  it('maps Canvas surface semantics to app tokens while keeping the named Canvas contract stable', () => {
    expect(surfaceSkin).toContain(
      '--canvas-surface-bg: color-mix(in srgb, var(--color-ib-surface) 98%, transparent)',
    )
    expect(surfaceSkin).toContain('--canvas-accent: var(--color-canvas-accent)')
    expect(surfaceSkin).toContain('--canvas-accent-hover: var(--color-canvas-accent-hover)')
    expect(surfaceSkin).toContain('--canvas-text-muted: var(--color-canvas-text-secondary)')
    expect(surfaceSkin).toContain('--canvas-surface-shadow:')
    expect(surfaceSkin).toContain('--canvas-floating-shadow: var(--canvas-surface-shadow)')
    expect(surfaceSkin).toContain('--flow-edge-running-duration: 0.5s')
  })

  it('keeps Canvas consumers on the named surface contract instead of direct app token aliases', () => {
    const consumers = [
      chromeSkin,
      nodeSkin,
      nodeToolSkin,
      nodeStageFrame,
      vueFlowAdapterSkin,
      canvasMiniMap,
      imageEditorToolRail,
      imageEditorLayerActions,
      imageEditorHistogram,
      maskEditorChrome,
    ]

    consumers.forEach((consumer) => {
      expect(consumer).toContain('--canvas-')
      expect(consumer).not.toContain('--color-canvas-')
      expect(consumer).not.toMatch(
        /var\(--canvas-(?:surface|accent|text)[^)]*,\s*(?:var\(--color-canvas|#[0-9a-fA-F]|rgba?\(|rgb\()/,
      )
    })
  })

  it('retains the focus, selected, active, disabled, loading, reduced-motion, and responsive contracts', () => {
    expect(chromeSkin).toContain('.canvas-chrome-board-icon-button:focus-visible')
    expect(chromeSkin).toContain('@media (prefers-reduced-motion: reduce)')
    expect(chromeSkin).toContain('max-width: calc(100vw - 32px)')
    expect(vueFlowAdapterSkin).toContain('edge.selected')
    expect(vueFlowAdapterSkin).toContain('edge:focus-visible')
    expect(canvasMiniMap).toContain('var(--canvas-minimap-node)')
    expect(nodeSkin).toContain('.workflow-node.running::before')
    expect(nodeSkin).toContain('animation: none;')
    expect(nodeToolSkin).toContain('.node-tool-output-count.active')
    expect(nodeToolSkin).toContain('.node-tool-output-count:disabled')
    expect(nodeStageFrame).toContain('.node-stage-frame.is-loading')
    expect(nodeStageFrame).toContain('.node-stage-frame.is-connection-drop-target')
    expect(nodeStageFrame).toContain('--node-stage-frame-emphasis-shadow-primary')
    expect(imageEditorToolRail).toContain("'is-disabled': getToolState(item.tool).disabled")
    expect(imageEditorLayerActions).toContain('@media (prefers-reduced-motion: reduce)')
    expect(maskEditorChrome).toContain(':disabled="directGenerating"')
  })
})
