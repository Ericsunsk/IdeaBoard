import '@testing-library/jest-dom/vitest'

// Font contract: loaded faces, CJK fallback chain, and zh/en wrapping geometry.
// Offline rendering and visual acceptance are verified manually.

import { page } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import '../app/style.css'

const SANS_ROLES = ['--font-sans', '--font-admin', '--font-display'] as const
const CJK_FALLBACKS = ['PingFang SC', 'Microsoft YaHei', 'Noto Sans CJK SC'] as const

function rootToken(name: string): string {
  return window.getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function bodyStack(): string {
  return window.getComputedStyle(document.body).fontFamily
}

function probeParagraph(text: string, widthPx: number): HTMLParagraphElement {
  const host = document.createElement('p')
  host.setAttribute('data-font-probe', '')
  host.style.cssText = `width:${widthPx}px;margin:0;padding:0;overflow-wrap:break-word;`
  host.textContent = text
  document.body.append(host)
  return host
}

beforeEach(async () => {
  await page.viewport(1280, 800)
})

afterEach(() => {
  for (const host of document.querySelectorAll('[data-font-probe]')) host.remove()
})

describe('font browser contract', () => {
  it('completes Inter Variable loading with font-display: swap semantics', async () => {
    // The variable face covers weights 400/500/600 from one file.
    // `load()` forces the fetch (page text alone may not reference the face
    // yet); `check()` then proves the loaded face serves each weight.
    for (const weight of ['400', '500', '600']) {
      const faces = await document.fonts.load(`${weight} 16px 'Inter Variable'`, 'Ag')
      expect(faces.length).toBeGreaterThan(0)
      for (const face of faces) {
        expect(face.family).toContain('Inter Variable')
        expect(face.status).toBe('loaded')
        expect(face.display).toBe('swap')
      }
      expect(document.fonts.check(`${weight} 16px 'Inter Variable'`, 'Ag')).toBe(true)
    }
    await document.fonts.ready
    expect(document.fonts.status).toBe('loaded')
  })

  it('binds the sans roles to Inter Variable with the explicit CJK chain', () => {
    for (const role of SANS_ROLES) {
      const value = rootToken(role)
      expect(value).toContain('Inter Variable')
      for (const fallback of CJK_FALLBACKS) expect(value).toContain(fallback)
    }
    // The body inherits the same explicit fallback chain.
    const stack = bodyStack()
    expect(stack).toContain('Inter Variable')
    for (const fallback of CJK_FALLBACKS) expect(stack).toContain(fallback)
  })

  it('wraps mixed zh/en text inside narrow containers without overflow', () => {
    const text = 'IdeaBoard 画布工作区Canvas Workspace 混合排版mixed text rendering 0123456789'
    const desktop = probeParagraph(text, 600)
    expect(desktop.scrollWidth).toBeLessThanOrEqual(desktop.clientWidth + 1)
    const narrow = probeParagraph(text, 200)
    expect(narrow.scrollWidth).toBeLessThanOrEqual(narrow.clientWidth + 1)
    // Wrapping actually happens: the narrow box grows taller than one line.
    expect(narrow.offsetHeight).toBeGreaterThan(desktop.offsetHeight)
  })

  it('scales long CJK strings with the root size (zoom equivalence)', async () => {
    const text = '这是一段用于验证缩放与换行的中文长文本内容'
    const host = probeParagraph(text, 300)
    host.style.fontSize = '1rem'
    const before = host.offsetHeight
    expect(before).toBeGreaterThan(0)
    document.documentElement.style.fontSize = '20px'
    try {
      await expect.poll(() => host.offsetHeight).toBeGreaterThan(before)
    } finally {
      document.documentElement.style.removeProperty('font-size')
    }
  })
})
