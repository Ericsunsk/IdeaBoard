import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAdminDialogPt, createAdminMenuPt, createAdminPopoverPt } from './adminOverlayPt'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Admin overlay Motion Policy', () => {
  it('maps shared Dialog and Popover primitives to semantic presence motion', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )

    expect(createAdminDialogPt({}).transition).toMatchObject({
      name: 'ib-dialog-motion',
      css: true,
    })
    expect(createAdminPopoverPt().transition).toMatchObject({
      name: 'ib-popover-motion',
      css: true,
    })
    expect(createAdminMenuPt({ rootClass: 'menu' }).transition).toMatchObject({
      name: 'ib-popover-motion',
      css: true,
    })
  })

  it('renders Admin overlays immediately under reduced motion', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )

    expect(createAdminDialogPt({}).transition).toMatchObject({ css: false })
    expect(createAdminPopoverPt().transition).toMatchObject({ css: false })
    expect(createAdminMenuPt({ rootClass: 'menu' }).transition).toMatchObject({ css: false })
  })
})
