import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const dialogMotionStyles = readFileSync(
  resolve(process.cwd(), 'src/shared/motion/dialogMotion.css'),
  'utf8',
)

describe('Dialog Motion CSS adapter', () => {
  it('maps dialog entry and exit to semantic Motion Policy tokens', () => {
    expect(dialogMotionStyles).toContain('var(--motion-enter-duration)')
    expect(dialogMotionStyles).toContain('var(--motion-enter-easing)')
    expect(dialogMotionStyles).toContain('var(--motion-exit-duration)')
    expect(dialogMotionStyles).toContain('var(--motion-exit-easing)')
    expect(dialogMotionStyles).toContain('var(--ib-dialog-motion-from-transform, none)')
  })

  it('removes dialog transforms under reduced motion', () => {
    expect(dialogMotionStyles).toContain('@media (prefers-reduced-motion: reduce)')
    expect(dialogMotionStyles).toContain('transition: none;')
    expect(dialogMotionStyles).toContain('transform: none;')
  })
})
