import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { render } from '../../../../test/render'
import CanvasGroupFramesOverlay from './CanvasGroupFramesOverlay.vue'
import type { CanvasGroupFrame } from '../../domain/graph/canvasGroupGeometry.ts'

const adapterSkinCss = readFileSync(
  path.join(
    process.cwd(),
    'src/features/canvas/presentation/styles/canvas-vue-flow-adapter-skin.css',
  ),
  'utf8',
)
const overlayVue = readFileSync(
  path.join(
    process.cwd(),
    'src/features/canvas/presentation/overlays/CanvasGroupFramesOverlay.vue',
  ),
  'utf8',
)

function createFrame(id: string, selected: boolean, left = 100, top = 80): CanvasGroupFrame {
  return {
    id: id as CanvasGroupFrame['id'],
    nodeIds: ['image-1', 'prompt-1'],
    selected,
    order: 0,
    bounds: {
      left,
      top,
      right: left + 320,
      bottom: top + 180,
      width: 320,
      height: 180,
    },
  }
}

describe('CanvasGroupFramesOverlay', () => {
  it('uses the cursor selection stroke width for the selected group frame ring', () => {
    expect(adapterSkinCss).toContain('--canvas-selection-stroke-width: 1px')
    expect(adapterSkinCss).toContain(
      '--canvas-group-frame-selected-ring-width: var(--canvas-selection-stroke-width)',
    )
    expect(adapterSkinCss).toContain(
      '--canvas-group-frame-selected-ring-color: var(--canvas-selection-group-border)',
    )
    expect(adapterSkinCss).toContain(
      'border: var(--canvas-selection-stroke-width) solid var(--canvas-selection-active-border)',
    )
    expect(adapterSkinCss).toContain(
      'border: var(--canvas-selection-stroke-width) solid var(--canvas-selection-group-border)',
    )
    expect(overlayVue).toContain('box-shadow: 0 0 0 var(--canvas-group-frame-selected-ring-width)')
    expect(overlayVue).toContain('var(--canvas-group-frame-selected-ring-color)')
  })

  it('projects persistent group frame bodies in the Vue Flow zoom plane', () => {
    const { container } = render(CanvasGroupFramesOverlay, {
      props: {
        frames: [createFrame('group-1', false), createFrame('group-2', true, 500, 120)],
      },
    })

    const frames = container.querySelectorAll<HTMLElement>('.canvas-group-frame-body')
    expect(frames).toHaveLength(2)
    const selectedFrame = frames[1]
    if (!selectedFrame) throw new Error('expected selected group frame')
    expect(selectedFrame.style.left).toBe('500px')
    expect(selectedFrame.style.top).toBe('120px')
    expect(selectedFrame.style.width).toBe('320px')
    expect(selectedFrame.style.height).toBe('180px')
    expect(selectedFrame.getAttribute('class')).toContain('is-selected')
  })

  it('renders selected group controls with resize zones only', () => {
    const { container } = render(CanvasGroupFramesOverlay, {
      props: {
        frames: [createFrame('group-1', true, 100, 80)],
      },
    })

    expect(container.querySelectorAll<HTMLElement>('.canvas-group-frame-controls')).toHaveLength(1)
    const resizeZones = container.querySelectorAll<HTMLElement>('.canvas-group-frame-resize-handle')
    expect(resizeZones).toHaveLength(8)
    expect(
      container.querySelectorAll<HTMLElement>('.canvas-group-frame-resize-handle.is-edge-handle'),
    ).toHaveLength(4)
    expect(
      container.querySelectorAll<HTMLElement>('.canvas-group-frame-resize-handle.is-corner-handle'),
    ).toHaveLength(4)
    expect(
      Array.from(resizeZones).map((zone) =>
        zone
          .getAttribute('class')
          ?.split(/\s+/)
          .find((className) => className.startsWith('handle-')),
      ),
    ).toEqual([
      'handle-nw',
      'handle-n',
      'handle-ne',
      'handle-e',
      'handle-se',
      'handle-s',
      'handle-sw',
      'handle-w',
    ])
    expect(Array.from(container.querySelectorAll('button'))).toEqual(Array.from(resizeZones))
  })

  it('does not render controls for unselected group frames', () => {
    const { container } = render(CanvasGroupFramesOverlay, {
      props: {
        frames: [createFrame('group-1', false)],
      },
    })

    expect(container.querySelectorAll<HTMLElement>('.canvas-group-frame-body')).toHaveLength(1)
    expect(container.querySelectorAll<HTMLElement>('.canvas-group-frame-controls')).toHaveLength(0)
  })
})
