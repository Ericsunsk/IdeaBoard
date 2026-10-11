import { describe, expect, it } from 'vitest'
import { render } from '../../../../test/render'
import CanvasGroupFramesOverlay from './CanvasGroupFramesOverlay.vue'
import type { CanvasGroupFrame } from '../../core/graph/canvasGroupGeometry.ts'

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
  // P5 retirement (source-d880bb7135287592-1, source-6afca5dc03ee30d2-1,
  // source-2f051ca903761065-1, source-e488601f448c871a-1,
  // source-521edb4dab2d2fa4-1, source-88b49d390f914daa-1,
  // source-cf435087c7b3738e-1): the "uses the cursor selection stroke
  // width for the selected group frame ring" source-string assertions moved
  // to the source-policy/CanvasGroupFramesOverlay.component/89c121de
  // governance script (scripts/checkCanvasUiStyleContracts.ts, rule
  // group-frame-ring), which asserts the adapter-skin declarations and the
  // overlay ring binding through parsed CSS/SFC linkage. The rendered
  // behavior tests below are retained-contract and unchanged.

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
