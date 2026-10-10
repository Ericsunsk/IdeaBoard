import { existsSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const domainDir = dirname(fileURLToPath(import.meta.url))
const toolsDir = resolve(domainDir, '..', 'tools')

const I18N_ROOTS = ['src/core/i18n', 'src/shared/i18n'] as const

function isForbiddenI18nTarget(resolved: string): boolean {
  return (
    I18N_ROOTS.some((root) => resolved === root || resolved.startsWith(`${root}/`)) &&
    resolved !== 'src/core/i18n/localeCore.ts'
  )
}

function resolveWebRelative(sourceFile: string, specifier: string): string | null {
  const webRoot = resolve(domainDir, '..', '..', '..', '..')
  if (!specifier.startsWith('.') && !specifier.startsWith('@/')) return null
  const resolved = specifier.startsWith('@/')
    ? resolve(webRoot, 'src', specifier.slice(2))
    : resolve(dirname(sourceFile), specifier)
  const candidates = [resolved, `${resolved}.ts`, join(resolved, 'index.ts')]
  const existing = candidates.find((candidate) => existsSync(candidate))
  return relative(webRoot, existing ?? resolved)
    .split('\\')
    .join('/')
}

describe('canvas tool core purity', () => {
  it('rejects relative and aliased i18n runtime/barrel imports while allowing localeCore', () => {
    const sourceFile = join(toolsDir, 'retouch', 'core', 'layers', 'layerStack.ts')
    for (const specifier of [
      '../../../../../../core/i18n',
      '@/core/i18n',
      '@/core/i18n/index.ts',
      '@/core/i18n/i18nRuntime',
      '@/core/i18n/userLocale',
      '../../../../../../shared/i18n/index.shim',
      '@/shared/i18n/userLocale.shim',
    ]) {
      expect(isForbiddenI18nTarget(resolveWebRelative(sourceFile, specifier)!)).toBe(true)
    }
    expect(isForbiddenI18nTarget(resolveWebRelative(sourceFile, '@/core/i18n/localeCore')!)).toBe(
      false,
    )
  })
})
