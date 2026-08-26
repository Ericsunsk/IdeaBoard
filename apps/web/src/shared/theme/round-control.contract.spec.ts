/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const roundControl = readSource('shared/theme/round-control.css')
const pageChrome = readSource('shared/theme/page-chrome.css')
const canvasChromeSkin = readSource('features/canvas/components/styles/canvas-chrome.css')

describe('Round Control surface ownership contract', () => {
  it('keeps Shared Round Control on global semantic color tokens', () => {
    expect(roundControl).toContain('--ib-round-control-border: var(--color-ib-border)')
    expect(roundControl).toContain('--ib-round-control-bg: var(--color-ib-surface)')
    expect(roundControl).toContain('--ib-round-control-color: var(--color-ib-text-muted)')
    expect(roundControl).toContain('--ib-round-control-focus-ring: var(--color-ib-focus-ring)')
    expect(roundControl).not.toMatch(/--color-(?:board|canvas)-/)
    expect(roundControl).not.toContain('--ib-control-')
  })

  it('lets Account chrome and Canvas tools provide complete surface state skins', () => {
    ;[
      '--ib-round-control-border:',
      '--ib-round-control-border-hover:',
      '--ib-round-control-bg:',
      '--ib-round-control-bg-hover:',
      '--ib-round-control-bg-disabled:',
      '--ib-round-control-color:',
      '--ib-round-control-color-hover:',
      '--ib-round-control-color-disabled:',
      '--ib-round-control-focus-ring:',
    ].forEach((token) => {
      expect(pageChrome).toContain(token)
      expect(canvasChromeSkin).toContain(token)
    })
    expect(pageChrome).toContain(
      '.ib-page-chrome-account > .ib-round-control.ib-page-chrome-control',
    )
    expect(canvasChromeSkin).toMatch(
      /\.canvas-chrome-icon-button\s*,\s*\.canvas-user-avatar-button/,
    )
  })
})
