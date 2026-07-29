import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPopoverMotionTransition } from './popoverMotion'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Popover Motion adapter', () => {
  it('uses semantic CSS presence motion by default', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )

    expect(createPopoverMotionTransition()).toEqual({
      name: 'ib-popover-motion',
      css: true,
    })
  })

  it('renders transient overlays immediately under reduced motion', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )

    expect(createPopoverMotionTransition()).toEqual({
      name: 'ib-popover-motion',
      css: false,
    })
  })
})
