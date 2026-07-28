/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasChromeStyles = readFileSync(
  resolve(process.cwd(), 'src/features/canvas/components/styles/canvas-chrome.css'),
  'utf8',
)

describe('Canvas Chrome Motion contract', () => {
  it('maps Chrome presence and feedback to Motion Policy tokens', () => {
    expect(canvasChromeStyles).toContain('var(--motion-enter-duration)')
    expect(canvasChromeStyles).toContain('var(--motion-enter-easing)')
    expect(canvasChromeStyles).toContain('var(--motion-exit-duration)')
    expect(canvasChromeStyles).toContain('var(--motion-exit-easing)')
    expect(canvasChromeStyles).toContain('var(--motion-feedback-duration)')
    expect(canvasChromeStyles).not.toContain('--canvas-chrome-fade-duration')
    expect(canvasChromeStyles).not.toContain('--canvas-chrome-slide-duration')
  })

  it('removes nonessential Chrome transforms and filters under reduced motion', () => {
    expect(canvasChromeStyles).toContain('@media (prefers-reduced-motion: reduce)')
    expect(canvasChromeStyles).toContain('transition: none;')
    expect(canvasChromeStyles).toContain('filter: none;')
    expect(canvasChromeStyles).toContain(
      '.canvas-chrome-region--left.is-hidden {\n    transform: translateY(-50%);',
    )
    expect(canvasChromeStyles).toContain(
      '.canvas-chrome-region--top.is-hidden,\n  .canvas-chrome-region--status.is-hidden,\n  .canvas-chrome-region--minimap.is-hidden {\n    transform: none;',
    )
  })
})
