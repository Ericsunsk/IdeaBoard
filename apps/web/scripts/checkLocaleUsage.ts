import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import en from '../src/core/i18n/locales/en/index.ts'
import { leafPaths } from '../src/core/i18n/localeParity.ts'

/**
 * Knip-style unused-locale report (warn-only): lists `en` leaf keys with no
 * static reference in `src/**`. Dead keys are NOT deleted here; locale parity
 * stays green. Delete a key only with a plan that updates all four bundles.
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
const KEY_PATTERN = /['"`]([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9_]+)+)['"`]/g

function listFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listFiles(full)
    return /\.(ts|vue)$/.test(entry) && !/\.spec\./.test(entry) ? [full] : []
  })
}

const keys = leafPaths(en as Record<string, unknown>)
const referenced = new Set<string>()
for (const file of listFiles(SRC_ROOT)) {
  const content = readFileSync(file, 'utf8')
  for (const match of content.matchAll(KEY_PATTERN)) referenced.add(match[1]!)
}

const unused = keys.filter((key) => !referenced.has(key))
if (unused.length > 0) {
  console.warn(`[locale-usage] ${unused.length} en keys have no static reference (warn-only):`)
  for (const key of unused.slice(0, 50)) console.warn(`  - ${key}`)
  if (unused.length > 50) console.warn(`  ... and ${unused.length - 50} more`)
} else {
  console.log('[locale-usage] every en key has at least one static reference.')
}
