import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAdminDialogPt, createAdminMenuPt, createAdminPopoverPt } from './adminOverlayPt'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Admin overlay Motion Policy', () => {
  it('keeps Dialog close controls free of the default gray hover and focus ring', () => {
    const closeButtonClass = String(
      (createAdminDialogPt({}).pcCloseButton as { root?: { class?: string } }).root?.class ?? '',
    )

    expect(closeButtonClass).toContain('hover:!bg-transparent')
    expect(closeButtonClass).toContain('focus-visible:!shadow-none')
    expect(closeButtonClass).toContain('focus-visible:!outline-none')
    expect(closeButtonClass).not.toContain('hover:!bg-ib-surface-muted')
    expect(closeButtonClass).not.toContain('focus-visible:!outline-admin-accent')
    expect(closeButtonClass).not.toContain('focus-visible:!shadow-[var(--shadow-ib-focus)]')
  })

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

  it('keeps Popover radius on the root and clips content inside it', () => {
    const pt = createAdminPopoverPt({ contentClass: '!p-0' })

    expect(pt.root).toMatchObject({
      class: expect.stringContaining('!rounded-[var(--radius-ib-overlay)]'),
    })
    expect(pt.root).toMatchObject({
      class: expect.stringContaining('!shadow-xl before:!hidden after:!hidden'),
    })
    expect(pt.content).toMatchObject({
      class: expect.stringContaining(
        '!overflow-clip !rounded-[calc(var(--radius-ib-overlay)-1px)]',
      ),
    })
    expect(pt.content).toMatchObject({ class: expect.stringContaining('!p-0') })
  })
})
