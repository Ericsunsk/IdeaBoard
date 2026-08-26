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
    ].forEach((token) => {
      expect(roundControl).toContain(token)
    })
    expect(pageChrome).not.toContain('--ib-round-control-')
    expect(canvasChromeSkin).not.toContain('--ib-round-control-border:')
    expect(canvasChromeSkin).not.toContain('--ib-page-chrome-account-')
  })
})
