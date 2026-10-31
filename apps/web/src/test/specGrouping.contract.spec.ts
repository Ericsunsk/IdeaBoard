// @vitest-environment node
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Spec suffix semantics gate for gallery/admin/shared (priority areas).
 *
 * - `unit`: deterministic pure logic, must not import `.vue` SFCs.
 * - `component`: observable SFC behavior via `src/test/render`.
 * - `contract`: API shape / policy fixtures, must not mock `generated-query`.
 * - `browser`: real-browser geometry only.
 *
 * Layout: gallery organizes into SaaS subdomains (`cards/`, `views/`, `controllers/`,
 * `range/`, `styling/`); admin/shared must split into `core/` / `session/` / `ui/`
 * subgroups (no flat specs in either root). Transitional exceptions carry reasons below;
 * new violations fail this lane.
 */

const SRC_ROOT = join(import.meta.dirname, '..')
const GALLERY_ROOT = join(SRC_ROOT, 'features', 'board', 'ui', 'gallery')
const ADMIN_SHARED_ROOT = join(SRC_ROOT, 'features', 'admin', 'shared')

const ALLOWED_SUFFIXES = [
  '.unit.spec.ts',
  '.component.spec.ts',
  '.contract.spec.ts',
  '.browser.spec.ts',
] as const

type Exception = { file: string; reason: string }

// Known transitional violations: unit specs that render SFCs. New code must
// use `.component.spec.ts` for SFC behavior instead of extending this list.
const UNIT_VUE_EXCEPTIONS: Exception[] = [
  {
    file: 'src/features/admin/shared/ui/adminControls.unit.spec.ts',
    reason:
      'Renders AdminButton/Input/Badge adapters via @vue/test-utils; migrate to component harness on next touch.',
  },
  {
    file: 'src/features/admin/shared/ui/form/AdminFormField.unit.spec.ts',
    reason:
      'Renders AdminFormField via src/test/render; rename to .component.spec.ts on next touch.',
  },
]

function listSpecFiles(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      out.push(...listSpecFiles(full))
      continue
    }
    if (entry.endsWith('.spec.ts')) out.push(full)
  }
  return out.sort()
}

function toSrcRelative(absolutePath: string): string {
  return `src/${relative(join(SRC_ROOT), absolutePath).split('\\').join('/')}`
}

function hasSuffix(file: string, suffix: string): boolean {
  return file.endsWith(suffix)
}

describe('spec suffix semantics (gallery/admin/shared)', () => {
  it('uses only the four canonical suffixes in priority areas', () => {
    const files = [...listSpecFiles(GALLERY_ROOT), ...listSpecFiles(ADMIN_SHARED_ROOT)]
    expect(files.length).toBeGreaterThan(0)
    for (const file of files) {
      const rel = toSrcRelative(file)
      expect(
        ALLOWED_SUFFIXES.some((suffix) => rel.endsWith(suffix)),
        `${rel} must end with one of ${ALLOWED_SUFFIXES.join(', ')}`,
      ).toBe(true)
    }
  })

  it('keeps admin/shared specs inside core/session/ui subgroups', () => {
    const flat = readdirSync(ADMIN_SHARED_ROOT).filter((entry) => entry.endsWith('.spec.ts'))
    expect(flat).toEqual([])
  })

  it('keeps gallery specs inside approved subdomains (no specs lingering in gallery root)', () => {
    const flat = readdirSync(GALLERY_ROOT).filter((entry) => entry.endsWith('.spec.ts'))
    expect(flat).toEqual([])
    const allowedSubdomains = new Set(['cards', 'views', 'controllers', 'range', 'styling'])
    const specs = listSpecFiles(GALLERY_ROOT)
    for (const file of specs) {
      const parts = relative(GALLERY_ROOT, file).split('/')
      expect(allowedSubdomains.has(parts[0]!)).toBe(true)
    }
  })

  it('restricts .vue imports in unit specs to the transitional allowlist', () => {
    const allowed = new Set(UNIT_VUE_EXCEPTIONS.map((entry) => entry.file))
    for (const exception of UNIT_VUE_EXCEPTIONS) {
      expect(exception.reason.trim().length).toBeGreaterThan(0)
    }
    const files = [...listSpecFiles(GALLERY_ROOT), ...listSpecFiles(ADMIN_SHARED_ROOT)].filter(
      (file) => hasSuffix(file, '.unit.spec.ts'),
    )
    const violations: string[] = []
    for (const file of files) {
      const rel = toSrcRelative(file)
      const content = readFileSync(file, 'utf8')
      const hasVueImport = /\.vue['"]/.test(content)
      if (hasVueImport && !allowed.has(rel)) violations.push(rel)
    }
    expect(violations).toEqual([])
    // Stale-allowlist check: every entry must still exist and still import .vue.
    for (const exception of UNIT_VUE_EXCEPTIONS) {
      const absolute = join(SRC_ROOT, exception.file.replace(/^src\//, ''))
      const content = readFileSync(absolute, 'utf8')
      expect(/\.vue['"]/.test(content), `${exception.file} no longer imports .vue; remove it`).toBe(
        true,
      )
    }
  })

  it('forbids generated-query mocks in contract specs', () => {
    const files = [...listSpecFiles(GALLERY_ROOT), ...listSpecFiles(ADMIN_SHARED_ROOT)].filter(
      (file) => hasSuffix(file, '.contract.spec.ts'),
    )
    for (const file of files) {
      const content = readFileSync(file, 'utf8')
      expect(content).not.toMatch(/generated-query/)
    }
  })
})
