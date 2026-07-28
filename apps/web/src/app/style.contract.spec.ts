import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const stylesheet = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')

describe('global scrollbar skin', () => {
  it('keeps the idle thumb readable on admin surfaces', () => {
    expect(stylesheet).toContain(
      '--color-ib-scrollbar-thumb: color-mix(in srgb, var(--color-ib-text) 58%, transparent);',
    )
  })
})

describe('Motion Policy CSS adapter', () => {
  it('maps semantic motion intents to the app timing and easing tokens', () => {
    expect(stylesheet).toContain('--motion-control-duration: var(--duration-ib-fast);')
    expect(stylesheet).toContain('--motion-enter-duration: var(--duration-ib-panel);')
    expect(stylesheet).toContain('--motion-exit-duration: var(--duration-ib-panel);')
    expect(stylesheet).toContain('--motion-feedback-duration: var(--duration-ib-fast);')
    expect(stylesheet).toContain('--motion-control-easing: var(--ease-ib-standard);')
  })

  it('globally stops semantic motion durations when reduced-motion is preferred', () => {
    expect(stylesheet).toContain('@media (prefers-reduced-motion: reduce)')
    expect(stylesheet).toContain('--motion-control-duration: 0ms;')
    expect(stylesheet).toContain('--motion-enter-duration: 0ms;')
    expect(stylesheet).toContain('--motion-exit-duration: 0ms;')
    expect(stylesheet).toContain('--motion-feedback-duration: 0ms;')
  })
})
