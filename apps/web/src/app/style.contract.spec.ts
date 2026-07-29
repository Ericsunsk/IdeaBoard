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
  it('owns semantic motion timing directly without legacy aliases', () => {
    expect(stylesheet).toContain('--motion-control-duration: 160ms;')
    expect(stylesheet).toContain('--motion-enter-duration: 220ms;')
    expect(stylesheet).toContain('--motion-exit-duration: 220ms;')
    expect(stylesheet).toContain('--motion-feedback-duration: 160ms;')

    for (const intent of ['control', 'enter', 'exit', 'feedback']) {
      expect(stylesheet).toContain(
        `--motion-${intent}-easing: cubic-bezier(0.2, 0.8, 0.2, 1);`,
      )
    }

    expect(stylesheet).not.toContain('--duration-ib-')
    expect(stylesheet).not.toContain('--ease-ib-')
  })

  it('globally stops semantic motion durations when reduced-motion is preferred', () => {
    expect(stylesheet).toContain('@media (prefers-reduced-motion: reduce)')
    expect(stylesheet).toContain('--motion-control-duration: 0ms;')
    expect(stylesheet).toContain('--motion-enter-duration: 0ms;')
    expect(stylesheet).toContain('--motion-exit-duration: 0ms;')
    expect(stylesheet).toContain('--motion-feedback-duration: 0ms;')
  })
})
