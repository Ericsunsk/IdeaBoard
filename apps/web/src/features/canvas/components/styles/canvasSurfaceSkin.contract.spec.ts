/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src/features/canvas')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const canvasScreen = readSource('workspace-ui/CanvasScreen.vue')
const surfaceSkin = readSource('components/styles/canvas-surface-skin.css')
const appStyle = readSource('../../app/style.css')
const chromeSkin = readSource('components/styles/canvas-chrome.css')
const styleContracts = readSource('components/styles/canvas-style-contracts.css')
const toolTooltipSkin = readSource('components/styles/canvas-tool-tooltip.css')
const nodeSkin = readSource('components/nodes/styles/node-shared.css')
const nodeToolSkin = readSource('components/nodes/styles/node-tool-buttons.css')
const nodeStageFrame = readSource('components/nodes/primitives/NodeStageFrame.vue')
const renderedImagePreviewCanvas = readSource(
  'image-tools/components/RenderedImagePreviewCanvas.vue',
)
const imageToolStagePresentation = readSource(
  'image-tools/components/ImageToolStagePresentation.vue',
)
const vueFlowAdapterSkin = readSource('components/styles/canvas-vue-flow-adapter-skin.css')
const canvasMiniMap = readSource('components/CanvasMiniMap.vue')
const nodeHandleRail = readSource('components/FlowCanvasNodeHandleRail.vue')
const retouchLayerActions = readSource('retouch/components/RetouchLayerActions.vue')
const retouchHistogram = readSource('retouch/components/styles/retouch-histogram-plot.css')
const inpaintChrome = readSource('inpaint/components/ImageInpaintChrome.vue')

describe('Canvas surface skin contract', () => {
  it('mounts one feature-local surface owner instead of declaring the stable token table in Canvas Screen', () => {
    expect(canvasScreen).toContain('canvas-surface-skin')
    expect(canvasScreen).not.toMatch(/--canvas-[a-z0-9-]+\s*:/)
    expect(canvasScreen).not.toMatch(/--flow-[a-z0-9-]+\s*:/)
    expect(surfaceSkin).toContain('.canvas-surface-skin')
    expect(surfaceSkin).toContain('html.canvas-overscroll-guard')
  })

  it('maps Canvas surface semantics to app tokens while keeping the named Canvas contract stable', () => {
    expect(surfaceSkin).toContain('--canvas-surface-bg: var(--color-canvas-surface-bg)')
    expect(surfaceSkin).toContain('--canvas-control-border-color: var(--color-canvas-neutral-line)')
    expect(appStyle).toMatch(/--color-canvas-neutral-line:\s*rgb\(177 177 183 \/ 65%\);/)
    expect(appStyle).toContain('--color-ib-page-chrome-border: var(--color-board-card-border)')
    expect(surfaceSkin).toContain('--flow-edge-color: var(--color-canvas-edge)')
    expect(surfaceSkin).toContain('--canvas-accent: var(--color-canvas-accent)')
    expect(surfaceSkin).toContain('--canvas-accent-hover: var(--color-canvas-accent-hover)')
    expect(surfaceSkin).toContain('--canvas-text-muted: var(--color-canvas-text-secondary)')
    expect(surfaceSkin).toContain('--canvas-shadow-floating:')
    expect(surfaceSkin).toContain('--flow-edge-running-duration: 0.5s')
    expect(nodeHandleRail).toContain('--handle-ring-color: var(--flow-edge-color)')
  })

  it('keeps Canvas consumers on the named surface contract instead of direct app token aliases', () => {
    const consumers = [
      chromeSkin,
      nodeSkin,
      nodeToolSkin,
      nodeStageFrame,
      vueFlowAdapterSkin,
      canvasMiniMap,
      retouchLayerActions,
      retouchHistogram,
      inpaintChrome,
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
    expect(chromeSkin).not.toContain('--ib-page-chrome-account-')
    expect(chromeSkin).not.toContain('.canvas-chrome-bordered-control')
    expect(chromeSkin).toContain('@media (prefers-reduced-motion: reduce)')
    expect(chromeSkin).toContain('max-width: calc(100vw - 32px)')
    expect(vueFlowAdapterSkin).toContain('edge.selected')
    expect(vueFlowAdapterSkin).toContain('edge:focus-visible')
    expect(canvasMiniMap).toContain('ib-page-chrome-surface canvas-minimap')
    expect(vueFlowAdapterSkin).not.toContain('var(--color-ib-page-chrome-border)')
    expect(vueFlowAdapterSkin).not.toContain('var(--color-ib-page-chrome-bg)')
    expect(canvasMiniMap).toContain('var(--canvas-minimap-node)')
    expect(nodeSkin).toContain('.workflow-node.running::before')
    expect(nodeSkin).toContain('animation: none;')
    expect(nodeToolSkin).toContain('.node-tool-output-count.active')
    expect(nodeToolSkin).toContain('.node-tool-output-count:disabled')
    expect(nodeStageFrame).toContain('.node-stage-frame.is-loading')
    expect(nodeStageFrame).toContain('.node-stage-frame.is-connection-drop-target')
    expect(nodeStageFrame).toContain('border: 0')
    expect(nodeStageFrame).toContain('.node-stage-frame::after')
    expect(nodeStageFrame).toContain('.node-stage-frame.is-active')
    expect(nodeStageFrame).toContain('--node-stage-frame-emphasis-shadow-primary')
    expect(inpaintChrome).toContain(':disabled="directGenerating"')
  })

  it('owns tooltips at the Canvas feature boundary without legacy node-only contracts', () => {
    expect(styleContracts).toContain("@import './canvas-tool-tooltip.css'")
    expect(toolTooltipSkin).toContain('.canvas-tool-tooltip')
    expect(toolTooltipSkin).toContain('.canvas-tool-tooltip-trigger')
    expect(toolTooltipSkin).toContain('--canvas-tool-tooltip-delay: 200ms')
    expect(toolTooltipSkin).toContain('transition-delay: var(--canvas-tool-tooltip-delay)')
    expect(nodeToolSkin).not.toContain('node-tool-tooltip')
    expect(nodeSkin).not.toContain('node-inline-range')
  })

  it('contains rendered adjustment previews within the existing image stage geometry', () => {
    const previewRule = renderedImagePreviewCanvas.match(
      /\.image-node-stage-image--preview\s*\{([\s\S]*?)\}/,
    )?.[1]

    expect(previewRule).toBeDefined()
    expect(previewRule).toContain('display: block')
    expect(previewRule).toContain('width: 100%')
    expect(previewRule).toContain('height: 100%')
    expect(previewRule).toContain('object-fit: contain')
    expect(previewRule).toContain('pointer-events: none')
    expect(imageToolStagePresentation).toContain(
      '<RenderedImagePreviewCanvas v-if="binding.renderedPreview"',
    )
    expect(imageToolStagePresentation).toContain('<slot v-else />')
    expect(imageToolStagePresentation).not.toContain('has-rendered-preview')
    expect(imageToolStagePresentation).not.toContain('visibility: hidden')
  })
})
