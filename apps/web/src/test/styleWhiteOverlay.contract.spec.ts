// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  rawWhiteOverlayPattern,
} from '../../scripts/checkStyleGovernance'

/*
 * raw-white-overlay pattern contract.
 *
 * Specs pin the exported pattern/validator fixtures only; repo-wide source
 * scans stay in `scripts/checkStyleGovernance.ts` (Web Quality / CI lane).
 * This keeps unit and governance lanes from double-scanning `src/**`.
 */

function matches(input: string): string[] {
  return [
    ...input.matchAll(new RegExp(rawWhiteOverlayPattern.source, rawWhiteOverlayPattern.flags)),
  ].map((match) => match[0])
}

describe('rawWhiteOverlayPattern contract', () => {
  it.each([
    'bg-white/10',
    'text-white/80',
    'border-white/15',
    'white_foo',
    'color-mix(in srgb, white 88%, transparent)',
    'color-mix(in srgb, white 80%, transparent)',
    'color-mix(in srgb, white 88.5%, transparent)',
  ])('flags raw white overlay: %s', (input) => {
    expect(matches(input).length).toBeGreaterThan(0)
  })

  it.each(['class="flex bg-white p-1"', 'class="text-white"', 'hover:bg-white'])(
    'flags bare utilities: %s',
    (input) => {
      expect(matches(input)).toContainEqual(expect.stringContaining('white'))
    },
  )

  it('counts both thumbnail veils', () => {
    const css = [
      'background: color-mix(in srgb, white 88%, transparent);',
      'background: color-mix(in srgb, white 88%, transparent);',
    ].join('\n')
    expect(matches(css)).toHaveLength(2)
  })

  it.each([
    'bg-card',
    'text-foreground',
    'var(--retouch-control-point-bg)',
    'var(--color-overlay-scrim-line)',
    'var(--primitive-white)',
    '--primitive-white: #ffffff;',
    'white Tier veil',
    'text-muted-foreground',
    'bg-white-foo',
  ])('passes semantic or unrelated usage: %s', (input) => {
    expect(matches(input)).toHaveLength(0)
  })
})
