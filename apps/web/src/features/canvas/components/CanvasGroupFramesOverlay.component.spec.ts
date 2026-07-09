import { describe, expect, it } from 'vitest'
import { render } from '../../../test/render'
import CanvasGroupFramesOverlay from './CanvasGroupFramesOverlay.vue'
import type { CanvasGroupFrame } from '../utils/canvasGroupGeometry'

function createFrame(
  id: string,
  selected: boolean,
  left = 100,
  top = 80,
): CanvasGroupFrame {
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
  it('projects persistent group frame bodies in the Vue Flow zoom plane', () => {
    const { container } = render(CanvasGroupFramesOverlay, {
      props: {
        frames: [createFrame('group-1', false), createFrame('group-2', true, 500, 120)],
      },
    })

    const frames = container.querySelectorAll<HTMLElement>('.canvas-group-frame-body')
    expect(frames).toHaveLength(2)
    expect(frames[1].style.left).toBe('500px')
    expect(frames[1].style.top).toBe('120px')
    expect(frames[1].style.width).toBe('320px')
    expect(frames[1].style.height).toBe('180px')
    expect(frames[1].getAttribute('class')).toContain('is-selected')
  })

  it('renders selected group controls with resize zones only', () => {
    const { container } = render(CanvasGroupFramesOverlay, {
      props: {
        frames: [createFrame('group-1', true, 100, 80)],
      },
    })

    expect(container.querySelectorAll<HTMLElement>('.canvas-group-frame-controls')).toHaveLength(
      1,
    )
    const resizeZones = container.querySelectorAll<HTMLElement>(
      '.canvas-group-frame-resize-handle',
    )
    expect(resizeZones).toHaveLength(8)
    expect(
      container.querySelectorAll<HTMLElement>(
        '.canvas-group-frame-resize-handle.is-edge-handle',
      ),
    ).toHaveLength(4)
    expect(
      container.querySelectorAll<HTMLElement>(
        '.canvas-group-frame-resize-handle.is-corner-handle',
      ),
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
    expect(container.querySelectorAll<HTMLElement>('.canvas-group-frame-controls')).toHaveLength(
      0,
    )
  })
})
