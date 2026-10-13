import { describe, expect, it } from 'vitest'
import {
  ADMIN_SELECT_SIDE_OFFSET,
  adminSelectContentClass,
  adminSelectItemClass,
  adminSelectItemCheckClass,
  adminSelectItemCompactClass,
  adminSelectItemIndicatorClass,
  adminSelectPanelClass,
  adminSelectTriggerActiveClass,
  adminSelectTriggerClass,
  adminSelectTriggerCompactClass,
  adminSelectTriggerIconClass,
  adminSelectViewportClass,
} from './adminSelectSkin'

describe('adminSelectSkin', () => {
  it('gives every trigger one radius, one height, and one border token', () => {
    expect(adminSelectTriggerClass).toContain('rounded-control')
    expect(adminSelectTriggerClass).toContain('h-9')
    expect(adminSelectTriggerClass).toContain('border-input')
  })

  it('exposes a visible focus indicator on every trigger', () => {
    // `focus-visible:outline-none` without a replacement is a WCAG 2.4.7
    // failure, so the ring is part of the skin rather than a caller concern.
    expect(adminSelectTriggerClass).toContain('focus-visible:outline-none')
    expect(adminSelectTriggerClass).toContain('focus-visible:ring-2')
  })

  it('keeps the disabled state readable and explains unavailability', () => {
    expect(adminSelectTriggerClass).toContain('disabled:cursor-not-allowed')
    expect(adminSelectTriggerClass).toContain('disabled:opacity-50')
  })

  it('scales the compact variant down without changing radius or border', () => {
    expect(adminSelectTriggerCompactClass).toContain('h-8')
    // Radius and border must not drift at compact size.
    expect(adminSelectTriggerCompactClass).not.toContain('rounded-')
    expect(adminSelectTriggerCompactClass).not.toContain('border-')
  })

  it('reuses the same radius for the content overlay and the panel', () => {
    expect(adminSelectContentClass).toContain('rounded-panel')
    expect(adminSelectPanelClass).toContain('rounded-panel')
  })

  it('animates every overlay in and out', () => {
    for (const overlay of [adminSelectContentClass, adminSelectPanelClass]) {
      expect(overlay).toContain('data-[state=open]:animate-in')
      expect(overlay).toContain('data-[state=closed]:animate-out')
      expect(overlay).toContain('data-[state=open]:fade-in-0')
    }
  })

  it('stacks every overlay at the same z-index', () => {
    expect(adminSelectContentClass).toContain('z-admin-dialog')
    expect(adminSelectPanelClass).toContain('z-admin-dialog')
  })

  it('detaches every overlay from its trigger by one distance', () => {
    // The old setup used 4 for selects and 6 for the filter panel.
    expect(ADMIN_SELECT_SIDE_OFFSET).toBe(4)
  })

  it('reuses one item skin, and only the compact variant differs', () => {
    expect(adminSelectItemClass).toContain('rounded-control')
    expect(adminSelectItemClass).toContain('focus:bg-muted')
    expect(adminSelectItemCompactClass).toContain('text-admin-meta')
    expect(adminSelectItemCompactClass).not.toContain('rounded-')
  })

  it('keeps the selected-state affordance in one place', () => {
    expect(adminSelectItemIndicatorClass).toContain('absolute')
    expect(adminSelectItemCheckClass).toContain('text-primary')
    expect(adminSelectTriggerIconClass).toContain('text-muted-foreground')
  })

  it('paints the active filter state with the accent, distinct from idle', () => {
    expect(adminSelectTriggerActiveClass).toContain('border-primary')
    expect(adminSelectTriggerActiveClass).not.toBe(adminSelectTriggerClass)
  })

  it('sizes the viewport from the trigger so the list never narrows', () => {
    expect(adminSelectViewportClass).toContain('min-w-[var(--reka-select-trigger-width)]')
  })
})
