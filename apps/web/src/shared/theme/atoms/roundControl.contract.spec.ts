/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const roundControl = readSource('shared/theme/atoms/round-control.css')
const pageChrome = readSource('shared/theme/atoms/page-chrome.css')
const canvasChromeSkin = readSource('features/canvas/presentation/styles/canvas-chrome.css')

describe('Round Control surface ownership contract', () => {
  it('exposes one geometry-neutral page chrome surface', () => {
    const surface = pageChrome.match(/\.page-chrome-surface\s*\{([^}]+)\}/)?.[1] ?? ''

    expect(pageChrome).toContain('.page-chrome-surface')
    expect(surface).toContain('border: 1px solid var(--color-page-chrome-border)')
    expect(surface).toContain('background: var(--color-page-chrome-bg)')
    expect(surface).toContain('box-shadow: var(--shadow-page-chrome)')
    expect(surface).toContain('backdrop-filter: blur(var(--blur-glass))')
    expect(surface).not.toMatch(/border-radius|width:|height:|padding:/)
  })

  it('keeps Shared Round Control on global semantic color tokens', () => {
    expect(roundControl).toContain('--round-control-border: var(--color-border)')
    expect(roundControl).toContain('--round-control-bg: var(--color-card)')
    expect(roundControl).toContain('--round-control-color: var(--color-muted-foreground)')
    expect(roundControl).toContain('--round-control-focus-ring: var(--color-ring)')
    expect(roundControl).toContain('--round-control-shadow: var(--primitive-shadow-sm)')
    expect(roundControl).not.toMatch(/--color-(?:board|canvas)-/)
    expect(roundControl).not.toContain('--control-')
  })

  it('keeps every page chrome control on one complete shared state skin', () => {
    ;[
      '--round-control-border: var(--color-page-chrome-border)',
      '--round-control-border-hover: var(--color-page-chrome-border-hover)',
      '--round-control-bg: var(--color-page-chrome-bg)',
      '--round-control-bg-hover: var(--color-page-chrome-bg-hover)',
      '--round-control-bg-disabled: var(--color-page-chrome-bg)',
      '--round-control-color: var(--color-page-chrome-color)',
      '--round-control-color-hover: var(--color-page-chrome-color-hover)',
      '--round-control-color-disabled: var(--color-page-chrome-color-disabled)',
      '--round-control-focus-ring: var(--color-page-chrome-focus-ring)',
      '--round-control-shadow: var(--shadow-page-chrome)',
    ].forEach((token) => {
      expect(roundControl).toContain(token)
    })
    expect(pageChrome).not.toContain('--round-control-')
    expect(canvasChromeSkin).not.toContain('--round-control-border:')
    expect(canvasChromeSkin).not.toContain('--page-chrome-account-')
  })
})
