import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createCanvasConnectionDropLifecycle,
  getConnectionEndPoint,
  resolveCanvasNodeIdAtPoint,
} from './connectionDropTarget'

describe('connection drop target', () => {
  afterEach(() => {
    document.body.replaceChildren()
    vi.restoreAllMocks()
  })

  it('reads the release point from mouse and touch events', () => {
    expect(getConnectionEndPoint(new MouseEvent('mouseup', { clientX: 12, clientY: 24 }))).toEqual({
      clientX: 12,
      clientY: 24,
    })

    const touch = { clientX: 48, clientY: 96 } as Touch
    const touchEvent = { changedTouches: [touch] } as unknown as TouchEvent
    expect(getConnectionEndPoint(touchEvent)).toEqual({ clientX: 48, clientY: 96 })
  })

  it('resolves the visually topmost Vue Flow node at the release point', () => {
    const node = document.createElement('div')
    node.dataset.flowNodeId = 'fusion-2'
    vi.spyOn(node, 'getBoundingClientRect').mockReturnValue({
      bottom: 200,
      height: 200,
      left: 0,
      right: 200,
      top: 0,
      width: 200,
      x: 0,
      y: 0,
      toJSON: () => undefined,
    })
    const content = document.createElement('div')
    node.append(content)
    document.body.append(node)

    vi.spyOn(document, 'elementFromPoint').mockReturnValue(content)

    expect(resolveCanvasNodeIdAtPoint(document, { clientX: 100, clientY: 120 })).toBe('fusion-2')
    expect(resolveCanvasNodeIdAtPoint(document, { clientX: 240, clientY: 120 })).toBeNull()
  })

  it('creates the aggregate target connection only when the drag ends over a node', () => {
    const node = document.createElement('div')
    node.dataset.flowNodeId = 'fusion-2'
    document.body.append(node)
    vi.spyOn(node, 'getBoundingClientRect').mockReturnValue({
      bottom: 200,
      height: 200,
      left: 0,
      right: 200,
      top: 0,
      width: 200,
      x: 0,
      y: 0,
      toJSON: () => undefined,
    })
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(node)
    const connect = vi.fn()
    const lifecycle = createCanvasConnectionDropLifecycle({ document, connect })

    expect(connect).not.toHaveBeenCalled()
    expect(
      lifecycle.onConnectEnd(new MouseEvent('mouseup', { clientX: 100, clientY: 120 }), {
        id: 'out:image',
        nodeId: 'image-1',
        type: 'source',
      }),
    ).toBe(true)
    expect(connect).toHaveBeenCalledOnce()
    expect(connect).toHaveBeenCalledWith({
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'fusion-2',
      targetHandle: 'in:__auto',
    })
  })
})
