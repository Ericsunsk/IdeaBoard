/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const src = (...parts: string[]) => readFileSync(resolve(process.cwd(), 'src', ...parts), 'utf8')

/**
 * Icon-only button accessible-name contract (no source change).
 *
 * NodeToolIconButton / NodeActionButton resolve `aria-label` as
 * `ariaLabel ?? tooltip`, so an icon-only caller that passes at least one of
 * the two always exposes a name; ActionButton forwards `ariaLabel` (and
 * `title`) to the underlying Button. Icon-only usage without either prop is a
 * caller bug — this spec pins the fallback chain so a refactor cannot silently
 * drop it. A repo-wide audit (all <NodeToolIconButton> / <NodeActionButton>
 * call sites) currently passes at least one naming prop.
 */
describe('Icon button accessible name contract', () => {
  it('resolves NodeToolIconButton aria-label from ariaLabel ?? tooltip', () => {
    const source = src('features/canvas/presentation/nodes/primitives', 'NodeToolIconButton.vue')
    expect(source).toContain(':aria-label="resolvedAriaLabel"')
    expect(source).toContain('props.ariaLabel ?? resolvedTooltip.value')
  })

  it('resolves NodeActionButton aria-label from ariaLabel ?? tooltip', () => {
    const source = src('features/canvas/presentation/nodes/primitives', 'NodeActionButton.vue')
    expect(source).toContain(':aria-label="resolvedAriaLabel"')
    expect(source).toContain('props.ariaLabel ?? resolvedTooltip.value')
  })
})
