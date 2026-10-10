import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import type { Plugin } from 'vite'
import { ROUTE_SEO } from '../src/core/seo/routeSeo.ts'
import { renderStaticSeoDocument } from '../src/core/seo/seoStaticHtml.ts'

const STATIC_PUBLIC_PATHS = ['terms', 'privacy', 'refund', 'acceptable-use'] as const

export function seoStaticHtmlVitePlugin(): Plugin {
  return {
    name: 'ideaboard-seo-static-html',
    apply: 'build',
    async writeBundle(outputOptions) {
      if (!outputOptions.dir) return
      const source = await readFile(join(outputOptions.dir, 'index.html'), 'utf8')

      await Promise.all(
        STATIC_PUBLIC_PATHS.map(async (key) => {
          const seo = ROUTE_SEO[key]
          if (!seo.canonicalPath) return
          const directory = join(outputOptions.dir!, seo.canonicalPath.slice(1))
          await mkdir(directory, { recursive: true })
          await writeFile(join(directory, 'index.html'), renderStaticSeoDocument(source, seo))
        }),
      )
    },
  }
}
