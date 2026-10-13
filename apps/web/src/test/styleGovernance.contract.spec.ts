// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { primitiveReferencePattern } from '../../scripts/checkStyleGovernance'

describe('primitive reference governance', () => {
  it.each([
    'var(--primitive-neutral-0)',
    'var(--primitive-neutral-0 )',
    'var( --palette-dark-0, var(--card))',
    'var(--primitive-neutral-0, var(--card))',
  ])('rejects direct primitive access in %s', (value) => {
    expect([...value.matchAll(primitiveReferencePattern)]).toHaveLength(1)
  })

  it('accepts semantic references and unrelated custom properties', () => {
    expect([
      ...'var(--card) var(--primary, currentColor) var(--component-primitive-neutral-0)'.matchAll(
        primitiveReferencePattern,
      ),
    ]).toHaveLength(0)
  })
})
