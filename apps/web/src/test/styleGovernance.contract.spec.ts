// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  darkVariantUtilityPattern,
  primitiveReferencePattern,
  rawClipboardPattern,
  rawDialogWidthPattern,
  rawMotionUtilityPattern,
  rawWhiteOverlayPattern,
} from '../../scripts/checkStyleGovernance'

describe('theme variant governance', () => {
  it.each(['dark:bg-black', 'hover:dark:text-white', 'dark:!border-white', 'dark:[color:red]'])(
    'rejects the dark utility %s',
    (utility) => {
      expect([...utility.matchAll(darkVariantUtilityPattern)]).toHaveLength(1)
    },
  )

  it('accepts color-mode object keys', () => {
    expect([..."{ dark: '', light: 'light' }".matchAll(darkVariantUtilityPattern)]).toHaveLength(0)
  })
})

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

describe('dialog width governance', () => {
  it('rejects raw 420px dialog widths outside dialogShell', () => {
    expect([...'w-[min(420px,calc(100vw-32px))]'.matchAll(rawDialogWidthPattern)]).toHaveLength(1)
  })

  it('accepts the dialogShell size contract', () => {
    expect([...'w-[min(30rem,calc(100vw-1.5rem))]'.matchAll(rawDialogWidthPattern)].length).toBe(0)
  })
})

describe('white overlay governance', () => {
  it.each(['border-white/15', 'border-white/95', 'white_86%'])(
    'rejects the raw white overlay value %s',
    (value) => {
      expect([...value.matchAll(rawWhiteOverlayPattern)]).toHaveLength(1)
    },
  )

  it('accepts overlay token references and whitespace utilities', () => {
    expect(
      [
        ...'border-overlay-scrim-line whitespace-nowrap border-[var(--color-overlay-scrim-line)]'.matchAll(
          rawWhiteOverlayPattern,
        ),
      ].length,
    ).toBe(0)
  })
})

describe('clipboard seam governance', () => {
  it('rejects direct clipboard access outside useAppClipboard', () => {
    expect([...'navigator?.clipboard.writeText(link)'.matchAll(rawClipboardPattern)]).toHaveLength(
      1,
    )
  })
})
