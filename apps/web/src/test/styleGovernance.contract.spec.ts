// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  primitiveReferencePattern,
  rawMotionUtilityPattern,
} from '../../scripts/checkStyleGovernance'

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

describe('motion utility governance', () => {
  it.each(['control', 'enter', 'exit', 'feedback'])(
    'accepts %s duration and easing policy tokens',
    (intent) => {
      const utilities = `duration-[var(--motion-${intent}-duration)] data-[state=closed]:ease-[var(--motion-${intent}-easing)]`
      expect([...utilities.matchAll(rawMotionUtilityPattern)]).toHaveLength(0)
    },
  )

  it('accepts explicit transition properties and CSS policy consumers', () => {
    const content = `
      transition-[opacity,transform]
      .control { transition: opacity var(--motion-control-duration) var(--motion-control-easing); }
    `
    expect([...content.matchAll(rawMotionUtilityPattern)]).toHaveLength(0)
  })

  it.each([
    'duration-[200ms]',
    'ease-[cubic-bezier(0.1,0.2,0.3,1)]',
    'animate-[spin_1s_linear_infinite]',
    'duration-[var(--local-duration)]',
    'ease-[var(--local-easing)]',
    'duration-[var(--motion-control-easing)]',
    'ease-[var(--motion-enter-duration)]',
    'duration-[var(--motion-control-duration,200ms)]',
    'duration-[calc(var(--motion-control-duration)+100ms)]',
  ])('rejects the arbitrary motion utility %s', (utility) => {
    expect([...utility.matchAll(rawMotionUtilityPattern)]).toHaveLength(1)
  })
})
