import { describe, expect, it } from 'vitest'

const sourceModules = import.meta.glob<string>('../../**/*.{ts,vue}', {
  eager: true,
  import: 'default',
  query: '?raw',
})
const directAppToastAccessPattern =
  /(?:import\s*\{[^}]*\buseAppToast\b[^}]*\}\s*from)|(?:\buseAppToast\s*\()/

function isAdminModule(path: string) {
  return path.startsWith('./') || /(?:^|\/)features\/admin\//.test(path)
}

function isAdminToastAdapter(path: string) {
  return path.endsWith('/shared/lib/adminToast.ts')
}

describe('admin feedback boundary', () => {
  it('scans the Admin source tree', () => {
    const adminPaths = Object.keys(sourceModules).filter(isAdminModule)

    expect(adminPaths).toContainEqual(expect.stringMatching(/\/shared\/lib\/adminToast\.ts$/))
  })

  it('keeps direct app toast access inside the shared transport adapter', () => {
    const offenders = Object.entries(sourceModules)
      .filter(([path]) => isAdminModule(path))
      .filter(([path]) => !isAdminToastAdapter(path))
      .filter(([path]) => !path.endsWith('.spec.ts'))
      .filter(([, source]) => directAppToastAccessPattern.test(source))
      .map(([path]) => path.replace(/^\.\//, ''))

    expect(offenders).toEqual([])
  })

  it('does not allow the retired global feedback tree or branch paths back into Admin', () => {
    const offenders = Object.entries(sourceModules)
      .filter(([path]) => isAdminModule(path))
      .filter(([path]) => !path.endsWith('.spec.ts'))
      .filter(([, source]) => /(?:useAdminFeedback|AdminFeedback\b)/.test(source))
      .map(([path]) => path.replace(/^\.\//, ''))

    expect(offenders).toEqual([])
  })
})
