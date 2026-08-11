import type { RouteSeo } from './routeSeo'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function replaceOrInsert(html: string, pattern: RegExp, replacement: string): string {
  if (pattern.test(html)) return html.replace(pattern, replacement)
  return html.replace('</head>', `    ${replacement}\n  </head>`)
}

export function renderStaticSeoDocument(html: string, seo: RouteSeo): string {
  if (!seo.canonicalPath) throw new Error('Static SEO documents require a canonical path')
  const canonical = `https://ideaboard.art${seo.canonicalPath}`
  const title = escapeHtml(seo.title)
  const description = escapeHtml(seo.description)
  const jsonLd = seo.jsonLd
    ? `<script id="ideaboard-route-jsonld" type="application/ld+json">${JSON.stringify(seo.jsonLd).replace(/</g, '\\u003c')}</script>`
    : ''
  const fallback = `<noscript id="ideaboard-route-fallback"><main><h1>${title}</h1><p>${description}</p><a href="/">IdeaBoard Home</a></main></noscript>`

  let output = html.replace(/<html\s+lang="[^"]*">/, '<html lang="en">')
  output = output.replace(/<title>[^<]*<\/title>/, `<title>${title}</title>`)
  output = replaceOrInsert(
    output,
    /<meta\s+name="description"[^>]*>/,
    `<meta name="description" content="${description}" />`,
  )
  output = replaceOrInsert(
    output,
    /<meta\s+name="robots"[^>]*>/,
    `<meta name="robots" content="${seo.robots}" />`,
  )
  output = replaceOrInsert(
    output,
    /<link\s+rel="canonical"[^>]*>/,
    `<link rel="canonical" href="${canonical}" />`,
  )
  output = replaceOrInsert(
    output,
    /<meta\s+property="og:title"[^>]*>/,
    `<meta property="og:title" content="${title}" />`,
  )
  output = replaceOrInsert(
    output,
    /<meta\s+property="og:description"[^>]*>/,
    `<meta property="og:description" content="${description}" />`,
  )
  output = replaceOrInsert(
    output,
    /<meta\s+property="og:url"[^>]*>/,
    `<meta property="og:url" content="${canonical}" />`,
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
