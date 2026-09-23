import { describe, expect, it } from 'vitest'
import { createNoopWorldCaptureToCanvas } from './worldCapture.ts'

describe('createNoopWorldCaptureToCanvas', () => {
  it('is never available and captures nothing', () => {
    const seam = createNoopWorldCaptureToCanvas()
    expect(seam.isAvailable).toBe(false)
    expect(seam.captureToCanvas({ sourceId: 'world-1' })).toBeNull()
    expect(seam.captureToCanvas({ sourceId: 'world-1', targetNodeId: 'node-1' })).toBeNull()
  })
})
