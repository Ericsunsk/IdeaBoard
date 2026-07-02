import { describe, expect, it } from 'vitest'

const sourceModules = import.meta.glob<string>('../../**/*.{ts,vue}', {
  eager: true,
  import: 'default',
  query: '?raw',
})
const directAppToastAccessPattern =
  /(?:import\s*\{[^}]*\buseAppToast\b[^}]*\}\s*from)|(?:\buseAppToast\s*\()/

function isFeedbackRecipeFile(path: string) {
  const fileName = path.split('/').pop() || ''
  return /^use[A-Z].*Feedback\.ts$/.test(fileName)
}

function isSharedToastAdapter(path: string) {
  return path.endsWith('/shared/composables/useAppToast.ts')
}

describe('admin feedback boundary', () => {
  it('keeps direct app toast access inside feedback recipes', () => {
    const offenders = Object.entries(sourceModules)
      .filter(([path]) => !isSharedToastAdapter(path))
      .filter(([path]) => !isFeedbackRecipeFile(path))
      .filter(([path]) => !path.endsWith('.spec.ts'))
      .filter(([, source]) => directAppToastAccessPattern.test(source))
      .map(([path]) => path.replace(/^\.\//, ''))

    expect(offenders).toEqual([])
  })
})
