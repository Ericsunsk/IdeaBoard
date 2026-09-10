/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const roundControl = readSource('shared/theme/round-control.css')
const pageChrome = readSource('shared/theme/page-chrome.css')
const canvasChromeSkin = readSource('features/canvas/presentation/styles/canvas-chrome.css')

describe('Round Control surface ownership contract', () => {
  it('exposes one geometry-neutral page chrome surface', () => {
    const surface = pageChrome.match(/\.ib-page-chrome-surface\s*\{([^}]+)\}/)?.[1] ?? ''

    expect(pageChrome).toContain('.ib-page-chrome-surface')
    expect(surface).toContain('border: 1px solid var(--color-ib-page-chrome-border)')
    expect(surface).toContain('background: var(--color-ib-page-chrome-bg)')
    expect(surface).toContain('box-shadow: var(--shadow-ib-page-chrome)')
    expect(surface).toContain('backdrop-filter: blur(var(--ib-glass-blur))')
    expect(surface).not.toMatch(/border-radius|width:|height:|padding:/)
  })

  it('keeps Shared Round Control on global semantic color tokens', () => {
    expect(roundControl).toContain('--ib-round-control-border: var(--color-ib-border)')
    expect(roundControl).toContain('--ib-round-control-bg: var(--color-ib-surface)')
    expect(roundControl).toContain('--ib-round-control-color: var(--color-ib-text-muted)')
    expect(roundControl).toContain('--ib-round-control-focus-ring: var(--color-ib-focus-ring)')
    expect(roundControl).toContain('--ib-round-control-shadow: var(--shadow-ib-control)')
    expect(roundControl).not.toMatch(/--color-(?:board|canvas)-/)
    expect(roundControl).not.toContain('--ib-control-')
  })

  it('keeps every page chrome control on one complete shared state skin', () => {
    ;[
      '--ib-round-control-border: var(--color-ib-page-chrome-border)',
      '--ib-round-control-border-hover: var(--color-ib-page-chrome-border-hover)',
      '--ib-round-control-bg: var(--color-ib-page-chrome-bg)',
      '--ib-round-control-bg-hover: var(--color-ib-page-chrome-bg-hover)',
      '--ib-round-control-bg-disabled: var(--color-ib-page-chrome-bg)',
      '--ib-round-control-color: var(--color-ib-page-chrome-color)',
      '--ib-round-control-color-hover: var(--color-ib-page-chrome-color-hover)',
      '--ib-round-control-color-disabled: var(--color-ib-page-chrome-color-disabled)',
      '--ib-round-control-focus-ring: var(--color-ib-page-chrome-focus-ring)',
      '--ib-round-control-shadow: var(--shadow-ib-page-chrome)',
    ].forEach((token) => {
      expect(roundControl).toContain(token)
    })
    expect(pageChrome).not.toContain('--ib-round-control-')
    expect(canvasChromeSkin).not.toContain('--ib-round-control-border:')
    expect(canvasChromeSkin).not.toContain('--ib-page-chrome-account-')
  })
})
