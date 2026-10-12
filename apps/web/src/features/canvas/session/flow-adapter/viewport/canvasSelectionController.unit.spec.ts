import { describe, expect, it, vi } from 'vitest'
import type { Node } from '@vue-flow/core'
import { createCanvasSelectionController } from './canvasSelectionController'

function node(id: string, isSelected = false): Node {
  return { id, position: { x: 0, y: 0 }, selected: isSelected } as Node &
    Partial<Pick<import('@vue-flow/core').GraphNode, 'selected'>>
}

describe('canvas selection controller', () => {
  it('clears selection and cancels an in-flight marquee', () => {
    const selected = [node('image-1', true)]
    const clear = vi.fn()
    const controller = createCanvasSelectionController({
      readElements: () => selected,
      clearElements: clear,
      addNodes: vi.fn(),
      findNode: vi.fn(),
    })
    controller.beginMarquee({ clientX: 0, clientY: 0, shiftKey: true })
    expect(controller.clear()).toBe(true)
    expect(controller.phase).toBe('idle')
    expect(clear).toHaveBeenCalledWith(selected)
  })

  it('restores nodes outside a shift marquee and removes duplicated selections', () => {
    const original = [node('image-1', true), node('image-2', true)]
    const clear = vi.fn()
    const restore = vi.fn()
    const controller = createCanvasSelectionController({
      readElements: () => original,
      clearElements: clear,
      addNodes: restore,
      findNode: (id) => original.find((item) => item.id === id),
    })
    controller.beginMarquee({ clientX: 0, clientY: 0, shiftKey: true })
    controller.endMarquee({ clientX: 10, clientY: 10 }, [node('image-1', true)])
    expect(clear).toHaveBeenCalledWith([expect.objectContaining({ id: 'image-1' })])
    expect(restore).toHaveBeenCalledWith([expect.objectContaining({ id: 'image-2' })])
  })

  it('does not toggle on a shift click without movement', () => {
    const clear = vi.fn()
    const controller = createCanvasSelectionController({
      readElements: () => [node('image-1', true)],
      clearElements: clear,
      addNodes: vi.fn(),
      findNode: vi.fn(),
    })
    controller.beginMarquee({ clientX: 4, clientY: 4, shiftKey: true })
    controller.endMarquee({ clientX: 4, clientY: 4 }, [])
    expect(clear).not.toHaveBeenCalled()
  })
})
