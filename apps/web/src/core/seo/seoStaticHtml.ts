import {
  buildRouteSeoTags,
  isNameMetaTag,
  metaTagAttribute,
  PUBLIC_SITE_ORIGIN,
  type RouteSeo,
  type RouteSeoMetaTag,
} from './routeSeo.ts'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function metaSelectorPattern(tag: RouteSeoMetaTag): RegExp {
  const attr = metaTagAttribute(tag)
  return new RegExp(`<meta\\s+${attr.name}="${attr.value}"[^>]*>`)
}

function renderMetaTag(tag: RouteSeoMetaTag): string {
  const attr = metaTagAttribute(tag)
  return `<meta ${attr.name}="${attr.value}" content="${escapeHtml(tag.content)}" />`
}

function replaceOrInsert(html: string, pattern: RegExp, replacement: string): string {
  if (pattern.test(html)) return html.replace(pattern, replacement)
  return html.replace('</head>', `    ${replacement}\n  </head>`)
}

export function renderStaticSeoDocument(html: string, seo: RouteSeo): string {
  if (!seo.canonicalPath) throw new Error('Static SEO documents require a canonical path')
  const tags = buildRouteSeoTags(seo)
  const canonical = tags.canonical ?? `${PUBLIC_SITE_ORIGIN}${seo.canonicalPath}`
  const title = escapeHtml(tags.title)
  const description = escapeHtml(seo.description)
  const jsonLd = tags.jsonLd
    ? `<script id="ideaboard-route-jsonld" type="application/ld+json">${JSON.stringify(tags.jsonLd).replace(/</g, '\\u003c')}</script>`
    : ''
  const fallback = `<noscript id="ideaboard-route-fallback"><main><h1>${title}</h1><p>${description}</p><a href="/">IdeaBoard Home</a></main></noscript>`

  let output = html.replace(/<html\s+lang="[^"]*">/, `<html lang="${tags.lang}">`)
  output = output.replace(/<title>[^<]*<\/title>/, `<title>${title}</title>`)
  for (const tag of tags.meta) {
    // `og:url` must resolve against the canonical URL even when the descriptor
    // falls back to the site origin (private routes never reach this renderer).
    const content = !isNameMetaTag(tag) && tag.property === 'og:url' ? canonical : tag.content
    const resolved: RouteSeoMetaTag = isNameMetaTag(tag)
      ? { name: tag.name, content }
      : { property: tag.property, content }
    output = replaceOrInsert(output, metaSelectorPattern(tag), renderMetaTag(resolved))
  }
  output = replaceOrInsert(
    output,
    /<link\s+rel="canonical"[^>]*>/,
    `<link rel="canonical" href="${canonical}" />`,
  )
  output = replaceOrInsert(
    output,
    /<script\s+id="ideaboard-route-jsonld"[^>]*>[\s\S]*?<\/script>/,
    jsonLd,
  )
  if (/<noscript\s+id="ideaboard-home-fallback"[^>]*>[\s\S]*?<\/noscript>/.test(output)) {
    output = output.replace(
      /<noscript\s+id="ideaboard-home-fallback"[^>]*>[\s\S]*?<\/noscript>/,
      fallback,
    )
  } else {
    output = output.replace('</body>', `    ${fallback}\n  </body>`)
  }

  return output
}
