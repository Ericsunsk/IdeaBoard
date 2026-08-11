import type { Plugin } from 'vite'
import { renderPublicHomeDocument } from './publicSiteStaticHtml'

export function publicSiteVitePlugin(): Plugin {
  return {
    name: 'ideaboard-public-site-static-html',
    transformIndexHtml(html) {
      return renderPublicHomeDocument(html)
    },
  }
}
