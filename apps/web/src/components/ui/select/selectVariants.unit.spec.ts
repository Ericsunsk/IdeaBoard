import { describe, expect, it } from 'vitest'
import { cn } from '@/lib/utils'
import {
  selectContentVariants,
  selectItemVariants,
  selectTriggerVariants,
  selectViewportVariants,
} from './selectVariants'

/**
 * Variant-less skins must be plain strings. A zero-variant `cva()` returns a
 * function, and `cn()`/`clsx()` silently drops functions — the base classes
 * never reached the DOM and the dropdowns rendered without position,
 * background, or shadow until this spec existed.
 */
describe('select variants', () => {
  it('exposes variant-less skins as strings that survive cn()', () => {
    for (const skin of [selectContentVariants, selectItemVariants, selectViewportVariants]) {
      expect(typeof skin).toBe('string')
      expect(cn(skin, 'extra-class')).toContain('extra-class')
    }
    expect(cn(selectContentVariants, 'x')).toContain('bg-popover')
    expect(cn(selectItemVariants, 'x')).toContain('cursor-default')
    expect(cn(selectViewportVariants, 'x')).toContain('p-1')
  })

  it('keeps the trigger variant on cva', () => {
    expect(typeof selectTriggerVariants).toBe('function')
    expect(selectTriggerVariants({ size: 'sm' })).toContain('h-8')
  })
})
