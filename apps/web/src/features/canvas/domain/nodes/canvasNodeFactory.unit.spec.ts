import { afterEach, describe, expect, it, vi } from 'vitest'
import { createCanvasNodeAt } from './canvasNodeFactory'

describe('canvasNodeFactory', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('creates integer node ids and keeps same-millisecond ids monotonic', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-09T00:00:00.000Z'))

    const first = createCanvasNodeAt('prompt', { x: 10, y: 20 })
    const second = createCanvasNodeAt('prompt', { x: 40, y: 20 })

    expect(Number.isInteger(first.id)).toBe(true)
    expect(Number.isInteger(second.id)).toBe(true)
    expect(second.id).toBe(first.id + 1)
  })
})
