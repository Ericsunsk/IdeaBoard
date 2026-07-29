import { afterEach, describe, expect, it, vi } from 'vitest'
import { getMotionCss, getMotionPreference, resolveMotionTiming } from './motionPolicy'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Motion Policy', () => {
  it('gives CSS and non-CSS adapters the same control semantics', () => {
    expect(getMotionCss('control')).toEqual({
      duration: 'var(--motion-control-duration)',
      easing: 'var(--motion-control-easing)',
    })

    expect(
      resolveMotionTiming('control', {
        preference: 'no-preference',
        readCssVariable: (name) => {
          const variables: Record<string, string> = {
            '--motion-control-duration': '160ms',
            '--motion-control-easing': 'cubic-bezier(0.2, 0.8, 0.2, 1)',
          }
          return variables[name]
        },
      }),
    ).toEqual({
      durationMs: 160,
      easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
      allowsTransform: true,
      allowsContinuousMotion: true,
    })
  })

  it('keeps state transitions available while honoring reduced-motion', () => {
    expect(
      resolveMotionTiming('enter', {
        preference: 'reduce',
        readCssVariable: () => '220ms',
      }),
    ).toEqual({
      durationMs: 0,
      easing: 'linear',
      allowsTransform: false,
      allowsContinuousMotion: false,
    })
  })

  it('reads the browser reduced-motion preference', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )
    expect(getMotionPreference()).toBe('no-preference')

    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    expect(getMotionPreference()).toBe('reduce')
  })

  it.each([
    ['milliseconds', '160ms', 160],
    ['seconds', '0.22s', 220],
    ['missing value', '', 0],
    ['invalid value', 'oopsms', 0],
    ['negative value', '-1ms', 0],
    ['infinite value', 'Infinityms', 0],
  ])('fails safe when the CSS duration reader returns %s', (_, duration, expectedDurationMs) => {
    expect(
      resolveMotionTiming('feedback', {
        preference: 'no-preference',
        readCssVariable: (name) =>
          name === '--motion-feedback-duration' ? duration : 'cubic-bezier(0.2, 0.8, 0.2, 1)',
      }),
    ).toMatchObject({ durationMs: expectedDurationMs })
  })
})
