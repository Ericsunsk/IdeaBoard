import { describe, expect, it, beforeEach } from 'vitest'
import { applyRouteSeo, resolveRouteSeo } from './routeSeo'
import { renderStaticSeoDocument } from './seoStaticHtml'

describe('route SEO contract', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
  })

  it('publishes the canonical Home metadata and JSON-LD', () => {
    const seo = applyRouteSeo({ meta: { seoKey: 'home' } })

    expect(seo.canonicalPath).toBe('/')
    expect(document.title).toBe('IdeaBoard | AI Canvas for Visual Designers')
    expect(document.documentElement.lang).toBe('en')
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe(
      'index,follow',
    )
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(
      'https://ideaboard.art/',
    )
    expect(document.querySelector('script#ideaboard-route-jsonld')?.textContent).toContain(
      'Organization',
    )
  })

  it('publishes Legal metadata and excludes private routes from indexing', () => {
    expect(resolveRouteSeo({ meta: { seoKey: 'terms' } }).robots).toBe('index,follow')
    const privateSeo = applyRouteSeo({ meta: {} })

    expect(privateSeo.robots).toBe('noindex,nofollow')
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe(
      'noindex,nofollow',
    )
    expect(document.querySelector('link[rel="canonical"]')).toBeNull()
    expect(document.querySelector('script#ideaboard-route-jsonld')).toBeNull()
  })

  it('renders route metadata into the build-time HTML shell', () => {
    const html = renderStaticSeoDocument(
      '<html lang="en"><head><title>Old</title></head><body><div id="app"></div></body></html>',
      resolveRouteSeo({ meta: { seoKey: 'privacy' } }),
    )

    expect(html).toContain('<title>Privacy Policy | IdeaBoard</title>')
    expect(html).toContain('href="https://ideaboard.art/privacy-policy"')
    expect(html).toContain('content="index,follow"')
    expect(html).toContain('id="ideaboard-route-jsonld"')
    expect(html).toContain('id="ideaboard-route-fallback"')
    expect(html).toContain('Privacy Policy for IdeaBoard')
    expect(html).not.toContain('id="ideaboard-home-fallback"')
    expect(html).not.toContain('Open IdeaBoard')
  })
})
