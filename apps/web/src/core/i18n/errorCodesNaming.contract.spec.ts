// @vitest-environment node
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const WEB_ROOT = join(import.meta.dirname, '..', '..', '..')
const LOCALES = ['en', 'zh-CN', 'es', 'ko'] as const

function readLocaleIndex(locale: string): string {
  return readFileSync(join(WEB_ROOT, 'src', 'core', 'i18n', 'locales', locale, 'index.ts'), 'utf8')
}

describe('errorCodes naming (import alias matches runtime key)', () => {
  it.each([...LOCALES])('mounts errorCodes shorthand in %s locale index', (locale) => {
    const source = readLocaleIndex(locale)
    expect(source).toContain("import errorCodes from './errors'")
    expect(source).not.toMatch(/^import errors from '\.\/errors'/m)
    expect(source).toMatch(/^\s*errorCodes,$/m)
    expect(source).not.toContain('errorCodes: errors')
    expect(source).not.toContain('errorCodes as errors')
  })

  it('mounts errorCodes shorthand in the i18n runtime', () => {
    const source = readFileSync(join(WEB_ROOT, 'src', 'core', 'i18n', 'i18nRuntime.ts'), 'utf8')
    expect(source).toContain('errorCodes,')
    expect(source).not.toContain('errorCodes: errors')
    expect(source).not.toMatch(/[^a-zA-Z]errors[,}]/)
  })

  it('keeps backend error-code copy out of billing UI copy', () => {
    for (const locale of LOCALES) {
      const source = readFileSync(
        join(WEB_ROOT, 'src', 'core', 'i18n', 'locales', locale, 'billing.ts'),
        'utf8',
      )
      expect(source).not.toContain('insufficientCredits')
    }
  })
})
