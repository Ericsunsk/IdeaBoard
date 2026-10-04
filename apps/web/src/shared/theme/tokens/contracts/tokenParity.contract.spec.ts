import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

describe('Theme Token Parity Contract', () => {
  const styleCss = readFileSync(join(import.meta.dirname, '../../../../app/style.css'), 'utf8')
  const darkCss = readFileSync(join(import.meta.dirname, '../dark.css'), 'utf8')

  it('ensures every dark color token exists in base @theme in style.css', () => {
    // Extract all --color-admin-*, --color-board-*, --color-canvas-*, --color-home-*, --color-ib-* declarations from dark.css
    const darkTokens = [
      ...darkCss.matchAll(/(--color-(?:admin|board|canvas|home|ib)-[a-z0-9-]+):/g),
    ].flatMap((m) => (m[1] ? [m[1]] : []))
    expect(darkTokens.length).toBeGreaterThan(0)

    for (const token of darkTokens) {
      const existsInBase = styleCss.includes(token)
      expect(
        existsInBase,
        `Token ${token} from dark.css should be declared in style.css @theme`,
      ).toBe(true)
    }
  })

  it('keeps dark token declarations unique within the dark scope', () => {
    const tokens = [...darkCss.matchAll(/(--color-(?:admin|board|canvas|home|ib)-[a-z0-9-]+):/g)].map(
      (match) => match[1],
    )
    const duplicates = [...new Set(tokens.filter((token, index) => tokens.indexOf(token) !== index))]
    expect(duplicates, 'dark.css must not redeclare the same token').toEqual([])
  })

  it('ensures marketing home tokens defined in style.css have corresponding dark tokens', () => {
    const homeTokens = [
      '--color-home-hero-text',
      '--color-home-hero-subtle',
      '--color-home-note-bg',
      '--color-home-note-border',
      '--color-home-note-text',
    ]

    for (const token of homeTokens) {
      expect(
        darkCss.includes(token),
        `Marketing home token ${token} must have a dark counterpart in dark.css`,
      ).toBe(true)
    }
  })

  it('ensures core canvas tokens defined in style.css have corresponding dark tokens', () => {
    const coreCanvasTokens = [
      '--color-canvas-workspace-bg',
      '--color-canvas-surface-bg',
      '--color-canvas-neutral-line',
      '--color-canvas-dot',
      '--color-canvas-edge',
      '--color-canvas-edge-selected',
      '--color-canvas-node-bg',
      '--color-canvas-node-border',
      '--color-canvas-text-primary',
      '--color-canvas-text-secondary',
      '--color-canvas-text-muted',
    ]

    for (const token of coreCanvasTokens) {
      expect(
        darkCss.includes(token),
        `Core canvas token ${token} must have a dark counterpart in dark.css`,
      ).toBe(true)
    }
  })

  it('ensures core board tokens defined in style.css have corresponding dark tokens', () => {
    const coreBoardTokens = [
      '--color-board-page',
      '--color-board-page-text',
      '--color-board-page-text-muted',
      '--color-board-surface',
      '--color-board-surface-strong',
      '--color-board-control-bg',
      '--color-board-card-border',
      '--color-board-card-bg',
      '--color-board-card-title',
      '--color-board-card-text',
    ]

    for (const token of coreBoardTokens) {
      expect(
        darkCss.includes(token),
        `Core board token ${token} must have a dark counterpart in dark.css`,
      ).toBe(true)
    }
  })

  it('ensures index.html includes synchronous anti-flash script before body renders', () => {
    const indexHtml = readFileSync(join(import.meta.dirname, '../../../../../index.html'), 'utf8')
    expect(indexHtml).toContain("localStorage.getItem('ideaboard-color-mode')")
    expect(indexHtml).toContain("window.matchMedia('(prefers-color-scheme: dark)')")
    expect(indexHtml).toContain("document.documentElement.classList.add('dark')")
    expect(indexHtml).toContain("mode === 'system'")
  })

  it('ensures index.css imports modular 3-tier tokens alongside legacy tokens', () => {
    const indexCss = readFileSync(join(import.meta.dirname, '../index.css'), 'utf8')
    expect(indexCss).toContain("@import './primitives.css';")
    expect(indexCss).toContain("@import './semantic-light.css';")
    expect(indexCss).toContain("@import './semantic-dark.css';")
    expect(indexCss).toContain("@import './canvas.css';")
    expect(indexCss).toContain("@import './light.css';")
    expect(indexCss).toContain("@import './dark.css';")
  })

  it('ensures 3-tier semantic tokens have complete parity between light and dark modes', () => {
    const semLightCss = readFileSync(join(import.meta.dirname, '../semantic-light.css'), 'utf8')
    const semDarkCss = readFileSync(join(import.meta.dirname, '../semantic-dark.css'), 'utf8')

    const lightTokens = [
      ...semLightCss.matchAll(/(--(?:color|blur)-[a-z0-9-]+):/g),
    ].map((m) => m[1])
    const darkTokens = [
      ...semDarkCss.matchAll(/(--(?:color|blur)-[a-z0-9-]+):/g),
    ].map((m) => m[1])

    expect(lightTokens.length).toBeGreaterThan(0)
    expect(darkTokens.length).toBeGreaterThan(0)

    // Every semantic token in light mode must exist in dark mode
    for (const token of lightTokens) {
      expect(
        darkTokens,
        `Semantic token ${token} declared in semantic-light.css must exist in semantic-dark.css`,
      ).toContain(token)
    }

    // Every semantic token in dark mode must exist in light mode
    for (const token of darkTokens) {
      expect(
        lightTokens,
        `Semantic token ${token} declared in semantic-dark.css must exist in semantic-light.css`,
      ).toContain(token)
    }
  })

  it('ensures canvas modular tokens have complete parity between light and dark modes', () => {
    const canvasCss = readFileSync(join(import.meta.dirname, '../canvas.css'), 'utf8')

    // Split canvas.css into :root block and .dark block
    const rootBlockMatch = canvasCss.match(/:root\s*\{([\s\S]*?)\}/)
    const darkBlockMatch = canvasCss.match(/(?:html\.dark,\s*\.dark|\.dark)\s*\{([\s\S]*?)\}/)

    expect(rootBlockMatch).not.toBeNull()
    expect(darkBlockMatch).not.toBeNull()

    const rootBlock = rootBlockMatch?.[1] ?? ''
    const darkBlock = darkBlockMatch?.[1] ?? ''
    const rootTokens = [...rootBlock.matchAll(/(--color-canvas-[a-z0-9-]+):/g)].flatMap((m) =>
      m[1] ? [m[1]] : [],
    )
    const darkTokens = [...darkBlock.matchAll(/(--color-canvas-[a-z0-9-]+):/g)].flatMap((m) =>
      m[1] ? [m[1]] : [],
    )

    expect(rootTokens.length).toBeGreaterThan(0)
    expect(darkTokens.length).toBeGreaterThan(0)

    for (const token of rootTokens) {
      expect(
        darkTokens,
        `Canvas token ${token} declared in :root must exist in dark block of canvas.css`,
      ).toContain(token)
    }

    for (const token of darkTokens) {
      expect(
        rootTokens,
        `Canvas token ${token} declared in dark block must exist in :root of canvas.css`,
      ).toContain(token)
    }
  })

  it('ensures physical primitives declare essential neutral, brand, radii, and shadow scales', () => {
    const primitivesCss = readFileSync(join(import.meta.dirname, '../primitives.css'), 'utf8')

    const requiredPrimitives = [
      '--primitive-neutral-0',
      '--primitive-neutral-50',
      '--primitive-neutral-100',
      '--primitive-neutral-800',
      '--primitive-neutral-900',
      '--primitive-neutral-950',
      '--primitive-violet-500',
      '--primitive-violet-650',
      '--primitive-radius-sm',
      '--primitive-radius-md',
      '--primitive-radius-lg',
      '--primitive-shadow-sm',
      '--primitive-shadow-md',
      '--primitive-shadow-lg',
    ]

    for (const token of requiredPrimitives) {
      expect(
        primitivesCss,
        `Primitive scale ${token} must be defined in primitives.css`,
      ).toContain(token)
    }
  })
})
