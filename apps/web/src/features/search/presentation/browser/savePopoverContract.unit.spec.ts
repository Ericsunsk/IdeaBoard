import { describe, expect, it } from 'vitest'
import { isSavePopoverController } from './savePopoverContract'

describe('search save popover contract', () => {
  it('recognizes the feature-owned popover controller shape', () => {
    expect(isSavePopoverController({ toggle: () => undefined, hide: () => undefined })).toBe(true)
  })

  it('rejects non-controller values without throwing', () => {
    expect(isSavePopoverController(null)).toBe(false)
    expect(isSavePopoverController(undefined)).toBe(false)
    expect(isSavePopoverController({ toggle: () => undefined })).toBe(false)
    expect(isSavePopoverController({ toggle: 'toggle', hide: () => undefined })).toBe(false)
  })
})
