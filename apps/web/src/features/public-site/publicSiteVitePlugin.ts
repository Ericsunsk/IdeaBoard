import type { Plugin } from 'vite'
import { renderPublicHomeDocument } from './publicSiteStaticHtml'

export function publicSiteVitePlugin(): Plugin {
  return {
    name: 'ideaboard-public-site-static-html',
    transformIndexHtml(html, ctx) {
      const path = ctx.path.replace(/\/index\.html$/, '') || '/'
      if (path !== '/') return html
      return renderPublicHomeDocument(html)
    },
  }
}
