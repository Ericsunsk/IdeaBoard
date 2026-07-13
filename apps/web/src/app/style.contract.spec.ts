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
