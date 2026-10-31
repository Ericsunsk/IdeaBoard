/**
 * Unified menu contract (second batch).
 *
 * Decision: command menus mount on `DropdownMenu` when they hang off a
 * trigger (account menu, board card menu), and on the `Popover` family
 * (`AdminPopover` / anchored `Popover`) when they need explicit x/y
 * positioning (admin row actions, anomaly radar is a dialog popover, logo
 * picker, canvas node context menu, library folder menu). No third
 * hand-rolled menu primitive: every menu shares the same motion and stacking
 * tokens below plus `handleMenuKeyboardNavigation` for arrow/Home/End support
 * and focus restoration on close.
 *
 * - Motion: `overlay-popover-motion` for reka-mounted menus (data-state
 *   driven); `popover-motion` Vue transition for absolutely-positioned
 *   context menus. Both ride the same `--motion-enter/exit` policy and honor
 *   `prefers-reduced-motion`.
 * - Stacking: user/canvas/library menus on `z-popover`; admin menus on
 *   `z-admin-dialog`. Canvas must not invent ad-hoc z values.
 */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { handleMenuKeyboardNavigation } from './menuKeyboardNavigation'

const overlayMenuMotionClass = 'overlay-popover-motion'
const userMenuStackClass = 'z-popover'
const adminMenuStackClass = 'z-admin-dialog'

const root = join(__dirname, '..', '..')

function read(relativePath: string): string {
  return readFileSync(join(root, relativePath), 'utf8')
}

describe('overlayMenuContract', () => {
  it('pins the shared motion and stacking tokens', () => {
    expect(overlayMenuMotionClass).toBe('overlay-popover-motion')
    expect(userMenuStackClass).toBe('z-popover')
    expect(adminMenuStackClass).toBe('z-admin-dialog')
  })

  it('moves focus with arrows, Home, and End inside role=menu', () => {
    document.body.innerHTML = `
      <div role="menu">
        <button role="menuitem">a</button>
        <button role="menuitem">b</button>
        <button role="menuitem">c</button>
      </div>
    `
    const menu = document.querySelector('[role="menu"]') as HTMLElement
    const items = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]'))
    items[0]!.focus()

    expect(
      handleMenuKeyboardNavigation(new KeyboardEvent('keydown', { key: 'ArrowDown' }), menu),
    ).toBe(true)
    expect(document.activeElement).toBe(items[1])

    expect(handleMenuKeyboardNavigation(new KeyboardEvent('keydown', { key: 'End' }), menu)).toBe(
      true,
    )
    expect(document.activeElement).toBe(items[2])

    expect(handleMenuKeyboardNavigation(new KeyboardEvent('keydown', { key: 'Home' }), menu)).toBe(
      true,
    )
    expect(document.activeElement).toBe(items[0])
  })

  it('keeps DropdownMenu and Admin popover menus on the unified motion/z contract', () => {
    const dropdown = read('components/ui/dropdown-menu/DropdownMenuContent.vue')
    expect(dropdown).toContain(overlayMenuMotionClass)
    expect(dropdown).toContain(userMenuStackClass)

    const adminContent = read('features/admin/shared/ui/popover/AdminPopoverContent.vue')
    expect(adminContent).toContain(overlayMenuMotionClass)
    expect(adminContent).toContain(adminMenuStackClass)

    const rowActions = read('features/admin/shared/ui/toolbar/AdminRowActionsPopover.vue')
    expect(rowActions).toContain(overlayMenuMotionClass)
    expect(rowActions).toContain(adminMenuStackClass)

    const logoPicker = read('features/admin/shared/ui/popover/AdminLogoPickerPopover.vue')
    expect(logoPicker).toContain(overlayMenuMotionClass)
    expect(logoPicker).toContain(adminMenuStackClass)
  })

  it('keeps canvas and library context menus on the same motion/z and keyboard contract', () => {
    const canvasMenu = read('features/canvas/ui/overlays/CanvasNodeContextMenu.vue')
    expect(canvasMenu).toContain(userMenuStackClass)
    expect(canvasMenu).not.toContain('z-[')
    expect(canvasMenu).toContain('handleMenuKeyboardNavigation')
    expect(canvasMenu).toContain('createPopoverMotionTransition')

    const libraryMenu = read('features/library/ui/LibraryFolderContextMenu.vue')
    expect(libraryMenu).toContain(userMenuStackClass)
    expect(libraryMenu).toContain('handleMenuKeyboardNavigation')
    expect(libraryMenu).toContain('createPopoverMotionTransition')

    const nodePopover = read('features/canvas/ui/nodes/primitives/tools/NodeToolPopover.vue')
    expect(nodePopover).toContain('createPopoverMotionTransition')
  })
})
