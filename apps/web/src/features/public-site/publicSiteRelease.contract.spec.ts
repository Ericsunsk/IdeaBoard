import { describe, expect, it } from 'vitest'
import {
  getPublishedPublicFacts,
  isExecutableWorkflowSlug,
  projectPublicContentLifecycle,
  PUBLIC_FACT_REGISTRY,
} from './publicFactRegistry'
import { buildPublicHomeJsonLd, PUBLIC_HOME_METADATA } from './publicSiteMetadata'
import { PUBLIC_HOME_STATIC_MARKUP, renderPublicHomeDocument } from './publicSiteStaticHtml'

describe('Public Site Release Contract', () => {
  it('publishes only canonical, reviewed content facts', () => {
    const published = getPublishedPublicFacts()

    expect(published.map((fact) => fact.path)).toEqual(['/'])
    expect(PUBLIC_FACT_REGISTRY.some((fact) => fact.status === 'draft')).toBe(true)
    expect(PUBLIC_FACT_REGISTRY.every((fact) => projectPublicContentLifecycle(fact))).toBe(true)
    expect(published.every((fact) => fact.path === '/' || !fact.path.endsWith('/'))).toBe(true)
    expect(
      PUBLIC_FACT_REGISTRY.flatMap((fact) => fact.workflowSlugs ?? []).every((slug) =>
        isExecutableWorkflowSlug(slug),
      ),
    ).toBe(true)
  })

  it('emits English homepage metadata and Organization/WebSite JSON-LD', () => {
    const html = renderPublicHomeDocument(
      '<!doctype html><html lang="zh-CN"><head><meta name="viewport" content="width=device-width, initial-scale=1.0" /><title>Idea Board</title></head><body><div id="app"></div></body></html>',
    )
    const jsonLd = buildPublicHomeJsonLd()

    expect(html).toContain('<html lang="en">')
    expect(html).toContain(`<title>${PUBLIC_HOME_METADATA.title}</title>`)
    expect(html).toContain(`<link rel="canonical" href="${PUBLIC_HOME_METADATA.canonical}" />`)
    expect(html).toContain('property="og:title"')
    expect(html).toContain('property="og:image"')
    expect(html).toContain('"@type":"Organization"')
    expect(html).toContain('"@type":"WebSite"')
    expect(html).toContain(PUBLIC_HOME_STATIC_MARKUP)
    expect(jsonLd['@graph']).toHaveLength(2)
  })
})
