import { join } from 'node:path'
import { SUPPORTED_LOCALES } from '../src/shared/i18n/localeCore'
import { diffLocaleBundles, leafPaths } from '../src/shared/i18n/localeParity'

/**
 * Locale parity guard: all User locale trees must contain exactly the same keys.
 * Reports missing locale, missing key, extra key, and shape mismatch.
 * English is canonical. Locale set is sourced from the i18n core manifest, and the
 * structural comparison reuses the same validator exercised by the unit tests.
 */

const LOCALES = SUPPORTED_LOCALES
const LOCALE_ROOT = join(import.meta.dirname, '..', 'src', 'shared', 'i18n', 'locales')

async function loadBundle(locale: string) {
  const mod = await import(join(LOCALE_ROOT, locale, 'index.ts'))
  return (mod as { default: Record<string, unknown> }).default
}

async function main() {
  const bundles = new Map<string, Record<string, unknown>>()
  const missingLocales: string[] = []

  for (const locale of LOCALES) {
    try {
      const bundle = await loadBundle(locale)
      if (!bundle) throw new Error('empty bundle')
      bundles.set(locale, bundle)
    } catch (e) {
      missingLocales.push(locale)
      console.error(`Missing locale bundle: ${locale} - ${(e as Error).message}`)
    }
  }

  if (missingLocales.length > 0) {
    console.error(`\nLocale parity check failed: missing locales: ${missingLocales.join(', ')}`)
    process.exit(1)
  }

  const enBundle = bundles.get('en')!
  const enKeys = leafPaths(enBundle).sort()

  let hasError = false
  const allErrors: string[] = []

  for (const locale of LOCALES) {
    if (locale === 'en') continue
    const bundle = bundles.get(locale)!
    const diff = diffLocaleBundles(enBundle, bundle)

    if (diff.missing.length > 0) {
      hasError = true
      allErrors.push(`\n[${locale}] Missing ${diff.missing.length} keys vs en:`)
      for (const k of diff.missing.slice(0, 50)) allErrors.push(`  - ${k}`)
      if (diff.missing.length > 50) allErrors.push(`  ... and ${diff.missing.length - 50} more`)
    }
    if (diff.extra.length > 0) {
      hasError = true
      allErrors.push(`\n[${locale}] Unexpected extra ${diff.extra.length} keys vs en:`)
      for (const k of diff.extra.slice(0, 50)) allErrors.push(`  + ${k}`)
      if (diff.extra.length > 50) allErrors.push(`  ... and ${diff.extra.length - 50} more`)
    }
    if (diff.shapeMismatches.length > 0) {
      hasError = true
      allErrors.push(`\n[${locale}] Shape mismatches vs en:`)
      for (const m of diff.shapeMismatches.slice(0, 50)) allErrors.push(`  ! ${m}`)
      if (diff.shapeMismatches.length > 50) allErrors.push(`  ... and ${diff.shapeMismatches.length - 50} more`)
    }
  }

  if (hasError) {
    console.error(['Locale parity check failed:', ...allErrors].join('\n'))
    process.exit(1)
  }

  console.log(`Locale parity check passed: ${enKeys.length} keys consistent across ${LOCALES.join(', ')}`)
}

main().catch((e) => {
  console.error('Locale parity check crashed:', e)
  process.exit(1)
})
