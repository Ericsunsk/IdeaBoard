// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { collectTouchProfileViolations } from '../../scripts/checkTouchTargetContract'

const theme = `${readFileSync(new URL('../theme/theme.css', import.meta.url), 'utf8')}\n${readFileSync(new URL('../theme/tokens/primitives.css', import.meta.url), 'utf8')}`
const targets = readFileSync(new URL('../theme/atoms/touch-target.css', import.meta.url), 'utf8')
const controls = readFileSync(
  new URL('../features/admin/shared/core/uiClasses.ts', import.meta.url),
  'utf8',
)

describe('touch profile routing contract', () => {
  it('routes existing ordinary controls without enlarging inline or precision axes', () => {
    expect(collectTouchProfileViolations(theme, targets, controls)).toEqual([])
  })

  it('accepts a coarse value uplift without a duplicate JavaScript pin', () => {
    const uplifted = theme.replace(/(--spacing-touch-target-coarse:\s*)[^;]+/, '$144px')
    expect(collectTouchProfileViolations(uplifted, targets, controls)).toEqual([])
  })

  it.each([
    ['--spacing-touch-target-min', '31px', 'Fine profile'],
    ['--spacing-touch-target-aa', '23px', 'Precision profile'],
    ['--spacing-touch-target-coarse', '31px', 'Coarse profile'],
    ['--spacing-touch-target-coarse', 'var(--missing-profile)', 'Coarse profile'],
  ])('rejects an invalid %s value', (token, value, message) => {
    const changed = theme.replace(new RegExp(`(${token}:\\s*)[^;]+`), `$1${value}`)
    expect(collectTouchProfileViolations(changed, targets, controls).join('\n')).toContain(message)
  })

  it('requires coarse targets to meet the declared fine-pointer floor', () => {
    const largerFine = theme.replace(/(--spacing-touch-target-min:\s*)[^;]+/, '$1100px')
    expect(collectTouchProfileViolations(largerFine, targets, controls).join('\n')).toContain(
      'Coarse profile',
    )
  })

  it('fails closed on a cyclic profile alias', () => {
    const changed = theme.replace(
      /(--spacing-touch-target-coarse:\s*)[^;]+/,
      '$1var(--spacing-touch-target-coarse)',
    )
    expect(collectTouchProfileViolations(changed, targets, controls).join('\n')).toContain(
      'Coarse profile',
    )
  })

  it.each(['(pointer: fine)', '(any-pointer: coarse)', '(min-width: 640px)'])(
    'rejects routing under %s instead of the primary coarse pointer',
    (condition) => {
      const changed = targets.replace('(pointer: coarse)', condition)
      const violations = collectTouchProfileViolations(theme, changed, controls)
      expect(violations.join('\n')).toContain('Unexpected coarse routing')
      expect(violations.join('\n')).toContain('Missing active coarse routing')
    },
  )

  it('rejects a token declaration with no existing ordinary target consumers', () => {
    const changed = targets.replaceAll(
      '--spacing-touch-target-coarse',
      '--spacing-touch-target-min',
    )
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'Missing active coarse routing',
    )
  })

  it('rejects a later override that disconnects an already bound coarse target', () => {
    const changed = `${targets}\n@media (pointer: coarse) {
      .hit-target-32::after { height: 32px; }
    }`
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'Pointer routing override disconnects',
    )
  })

  it('rejects removal of the retained precision hit area', () => {
    const changed = targets.replace('width: max(100%, var(--spacing-touch-target-aa));', '')
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'Missing retained fine/precision geometry',
    )
  })

  it.each(['adminCompactIconButtonClass', 'adminTableRowActionButtonClass'])(
    'requires %s to keep its real-box ordinary binding',
    (name) => {
      const changed = controls.replace(
        new RegExp(`(${name}\\s*=\\s*['"])hit-target-control `),
        '$1',
      )
      expect(changed).not.toBe(controls)
      expect(collectTouchProfileViolations(theme, targets, changed).join('\n')).toContain(name)
    },
  )

  it('rejects inline-axis expansion that can cover neighbouring row targets', () => {
    const changed = targets.replace(
      'width: 100%;',
      'width: max(100%, var(--spacing-touch-target-min));',
    )
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'must not expand the inline axis',
    )
  })

  it('keeps precision handles independent of ordinary pointer profiles', () => {
    const changed = targets.replaceAll('--spacing-touch-target-aa', '--spacing-touch-target-coarse')
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'Precision handles must retain',
    )
  })

  it('rejects a pseudo overlay on real-box controls that must grow in layout', () => {
    const changed = targets.replace('.hit-target-control {', '.hit-target-control::after {')
    expect(collectTouchProfileViolations(theme, changed, controls).join('\n')).toContain(
      'without an overlapping pseudo-element',
    )
  })
})
