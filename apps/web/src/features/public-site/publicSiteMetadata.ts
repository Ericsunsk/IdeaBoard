import { PUBLIC_HOME_FACT, PUBLIC_SITE_ORIGIN } from './publicFactRegistry'

export const PUBLIC_HOME_METADATA = {
  title: 'IdeaBoard | AI Canvas for Visual Designers',
  description:
    'IdeaBoard is an AI Canvas for Visual Designers and Pattern Designers to explore, compare, and refine image directions in one connected workspace.',
  canonical: PUBLIC_SITE_ORIGIN,
  ogImage: `${PUBLIC_SITE_ORIGIN}/favicon.svg`,
  locale: 'en_US',
} as const

export function buildPublicHomeJsonLd() {
  return {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'Organization',
        '@id': `${PUBLIC_SITE_ORIGIN}/#organization`,
        name: 'IdeaBoard',
        url: PUBLIC_SITE_ORIGIN,
        logo: `${PUBLIC_SITE_ORIGIN}/favicon.svg`,
      },
      {
        '@type': 'WebSite',
        '@id': `${PUBLIC_SITE_ORIGIN}/#website`,
        name: 'IdeaBoard',
        url: PUBLIC_SITE_ORIGIN,
        description: PUBLIC_HOME_METADATA.description,
        inLanguage: PUBLIC_HOME_METADATA.locale,
        publisher: { '@id': `${PUBLIC_SITE_ORIGIN}/#organization` },
      },
    ],
  }
}

export function serializePublicHomeJsonLd(): string {
  return JSON.stringify(buildPublicHomeJsonLd()).replace(/</g, '\\u003c')
}

export function getPublicHomeFacts() {
  return PUBLIC_HOME_FACT
}
