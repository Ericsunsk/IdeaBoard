export const PUBLIC_SITE_ORIGIN = 'https://ideaboard.art'

export type RouteSeoKey = 'home' | 'terms' | 'privacy' | 'refund' | 'acceptable-use'

export type RouteSeo = {
  title: string
  description: string
  canonicalPath: string | null
  robots: 'index,follow' | 'noindex,nofollow'
  jsonLd?: Record<string, unknown>
}

export type RouteSeoLocation = {
  meta: { seoKey?: RouteSeoKey }
}

const organizationId = `${PUBLIC_SITE_ORIGIN}/#organization`

const organizationJsonLd = {
  '@type': 'Organization',
  '@id': organizationId,
  name: 'IdeaBoard',
  url: PUBLIC_SITE_ORIGIN,
  logo: `${PUBLIC_SITE_ORIGIN}/favicon.svg`,
}

const websiteJsonLd = {
  '@type': 'WebSite',
  '@id': `${PUBLIC_SITE_ORIGIN}/#website`,
  name: 'IdeaBoard',
  url: PUBLIC_SITE_ORIGIN,
  inLanguage: 'en',
  publisher: { '@id': organizationId },
}

function buildHomeJsonLd(description: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@graph': [organizationJsonLd, { ...websiteJsonLd, description }],
  }
}

function buildLegalJsonLd(seo: { title: string; description: string; canonicalPath: string }) {
  const url = `${PUBLIC_SITE_ORIGIN}${seo.canonicalPath}`
  return {
    '@context': 'https://schema.org',
    '@graph': [
      organizationJsonLd,
      {
        '@type': 'WebPage',
        '@id': url,
        url,
        name: seo.title,
        description: seo.description,
        inLanguage: 'en',
        isPartOf: { '@id': `${PUBLIC_SITE_ORIGIN}/#website` },
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {
            '@type': 'ListItem',
            position: 1,
            name: 'IdeaBoard',
            item: PUBLIC_SITE_ORIGIN,
          },
          {
            '@type': 'ListItem',
            position: 2,
            name: seo.title,
            item: url,
          },
        ],
      },
    ],
  }
}

const baseRouteSeo = {
  home: {
    title: 'IdeaBoard | AI Canvas for Visual Designers',
    description:
      'IdeaBoard is an AI Canvas for Visual Designers to explore, compare, and refine image directions in one connected workspace.',
    canonicalPath: '/',
    robots: 'index,follow',
  },
  terms: {
    title: 'Terms of Service | IdeaBoard',
    description:
      'Terms of Service for IdeaBoard, including accounts, Ideas, purchases, user content, and acceptable use.',
    canonicalPath: '/terms',
    robots: 'index,follow',
  },
  privacy: {
    title: 'Privacy Policy | IdeaBoard',
    description:
      'Privacy Policy for IdeaBoard, covering account information, usage data, payments, security, and data rights.',
    canonicalPath: '/privacy-policy',
    robots: 'index,follow',
  },
  refund: {
    title: 'Refund Policy | IdeaBoard',
    description:
      'Refund Policy for Ideas purchased through IdeaBoard Checkout, including eligibility and processing.',
    canonicalPath: '/refund-policy',
    robots: 'index,follow',
  },
  'acceptable-use': {
    title: 'Acceptable Use & Copyright Policy | IdeaBoard',
    description:
      'Acceptable Use and Copyright Policy for IdeaBoard content, prompts, uploaded media, shared Boards, and generated output.',
    canonicalPath: '/acceptable-use',
    robots: 'index,follow',
  },
} satisfies Record<RouteSeoKey, Omit<RouteSeo, 'jsonLd'>>

export const ROUTE_SEO: Record<RouteSeoKey, RouteSeo> = {
  home: { ...baseRouteSeo.home, jsonLd: buildHomeJsonLd(baseRouteSeo.home.description) },
  terms: { ...baseRouteSeo.terms, jsonLd: buildLegalJsonLd(baseRouteSeo.terms) },
  privacy: { ...baseRouteSeo.privacy, jsonLd: buildLegalJsonLd(baseRouteSeo.privacy) },
  refund: { ...baseRouteSeo.refund, jsonLd: buildLegalJsonLd(baseRouteSeo.refund) },
  'acceptable-use': {
    ...baseRouteSeo['acceptable-use'],
    jsonLd: buildLegalJsonLd(baseRouteSeo['acceptable-use']),
  },
}

const PRIVATE_ROUTE_SEO: RouteSeo = {
  title: 'IdeaBoard Workspace',
  description: 'IdeaBoard authenticated workspace.',
  canonicalPath: null,
  robots: 'noindex,nofollow',
}

export type RouteSeoMetaTag =
  { name: string; content: string } | { property: string; content: string }

export type RouteSeoTags = {
  title: string
  lang: string
  canonical: string | null
  meta: RouteSeoMetaTag[]
  jsonLd?: Record<string, unknown> | undefined
}

export function isNameMetaTag(tag: RouteSeoMetaTag): tag is { name: string; content: string } {
  return 'name' in tag
}

export function metaTagAttribute(tag: RouteSeoMetaTag): {
  name: 'name' | 'property'
  value: string
} {
  return isNameMetaTag(tag)
    ? { name: 'name', value: tag.name }
    : { name: 'property', value: tag.property }
}

/**
 * Single tag descriptor shared by the client router guard (`applyRouteSeo`)
 * and the build-time static HTML renderer (`renderStaticSeoDocument`).
 * Add or rename a tag here — never in only one of the two sinks.
 */
export function buildRouteSeoTags(seo: RouteSeo): RouteSeoTags {
  const canonical = seo.canonicalPath ? `${PUBLIC_SITE_ORIGIN}${seo.canonicalPath}` : null
  return {
    title: seo.title,
    lang: 'en',
    canonical,
    meta: [
      { name: 'description', content: seo.description },
      { name: 'robots', content: seo.robots },
      { property: 'og:title', content: seo.title },
      { property: 'og:description', content: seo.description },
      { property: 'og:type', content: 'website' },
      { property: 'og:url', content: canonical ?? PUBLIC_SITE_ORIGIN },
      { property: 'og:site_name', content: 'IdeaBoard' },
    ],
    ...(seo.jsonLd ? { jsonLd: seo.jsonLd } : {}),
  }
}

function upsertMetaTag(tag: RouteSeoMetaTag) {
  const attr = metaTagAttribute(tag)
  const selector = `meta[${attr.name}="${attr.value}"]`
  let element = document.head.querySelector<HTMLMetaElement>(selector)
  if (!element) {
    element = document.createElement('meta')
    element.setAttribute(attr.name, attr.value)
    document.head.appendChild(element)
  }
  element.setAttribute('content', tag.content)
}

function upsertCanonical(href: string) {
  let link = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'canonical'
    document.head.appendChild(link)
  }
  link.href = href
}

function upsertJsonLd(jsonLd?: Record<string, unknown>) {
  const id = 'ideaboard-route-jsonld'
  let script = document.head.querySelector<HTMLScriptElement>(`script#${id}`)
  if (!jsonLd) {
    script?.remove()
    return
  }
  if (!script) {
    script = document.createElement('script')
    script.id = id
    script.type = 'application/ld+json'
    document.head.appendChild(script)
  }
  script.textContent = JSON.stringify(jsonLd).replace(/</g, '\\u003c')
}

export function resolveRouteSeo(route: RouteSeoLocation): RouteSeo {
  const key = route.meta.seoKey
  if (key) return ROUTE_SEO[key]
  return PRIVATE_ROUTE_SEO
}

export function applyRouteSeo(route: RouteSeoLocation): RouteSeo {
  const seo = resolveRouteSeo(route)
  const tags = buildRouteSeoTags(seo)

  document.documentElement.lang = tags.lang
  document.title = tags.title
  for (const tag of tags.meta) upsertMetaTag(tag)
  if (tags.canonical) {
    upsertCanonical(tags.canonical)
  } else {
    document.head.querySelector('link[rel="canonical"]')?.remove()
  }
  upsertJsonLd(tags.jsonLd)

  return seo
}
