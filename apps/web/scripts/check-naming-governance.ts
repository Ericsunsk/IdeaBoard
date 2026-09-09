import { readdirSync, statSync } from 'node:fs'
import { basename, join } from 'node:path'

/**
 * Naming governance ratchet (issue #684).
 *
 * Enforces, under apps/web/src:
 * 1. Directory names are kebab-case (BCP 47 locale codes directly under a
 *    `locales/` directory are exempt: they follow an external standard).
 * 2. Vue SFC files are PascalCase.vue.
 * 3. Non-component TypeScript modules are camelCase.ts. Generated Orval
 *    output (`** /generated-query/**`) and ambient declarations (`*.d.ts`)
 *    are exempt. A PascalCase-prefixed spec is allowed only when it tests a
 *    colocated PascalCase SFC (e.g. `Foo.component.spec.ts` next to `Foo.vue`).
 * 4. Anti-stutter (features/ only, ancestor-level per WP-6): a module must
 *    not repeat any SUBDOMAIN ancestor directory name as a word prefix. The
 *    feature root itself is the feature namespace and stays allowed (e.g.
 *    canvasFrameProbe under features/canvas). Excepted: directory-barrel
 *    convention (`controller/controller.ts`), `index.ts`, specs, SFCs, and a
 *    single trailing artifact-kind remainder (`Contract`, `Ref`, `Upload`,
 *    `Persistence`).
 *
 * Ratchet: `legacyAllowances` tracks pre-existing violations. Any violation
 * NOT in the map fails the check; an allowance whose file no longer violates
 * prints a stale warning so it can be removed (issues #686-#689).
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')

type ViolationKind = 'directory' | 'vue-sfc' | 'ts-module' | 'anti-stutter'

type Violation = {
  kind: ViolationKind
  path: string
  detail: string
}

const SPEC_SUFFIX_PATTERN = /\.(unit|contract|component|browser)\.spec$/
const KIND_REMAINDER_ALLOWLIST = new Set(['Contract', 'Ref', 'Upload', 'Persistence'])

const legacyAllowances: Record<string, string> = {}

function splitWords(value: string): string[] {
  return value
    .replace(/[-_]+/g, ' ')
    .split(/(?=[A-Z])|\s+/)
    .filter(Boolean)
    .map((word) => word.toLowerCase())
}

function siblingVueExists(dir: string, prefix: string): boolean {
  try {
    return readdirSync(dir).some(
      (file) => file === `${prefix}.vue` || file.startsWith(`${prefix}.`),
    )
  } catch {
    return false
  }
}

function checkTsModule(relativePath: string, absoluteDir: string, fileName: string): Violation | null {
  const stem = fileName.slice(0, -'.ts'.length)
  const specMatch = stem.match(/^(.*)\.(unit|contract|component|browser)\.spec$/)
  const base = specMatch ? (specMatch[1] ?? stem) : stem
  const segments = base.split('.')
  const firstSegment = segments[0] ?? ''
  const allCamel = segments.every((segment) => /^[a-z][A-Za-z0-9]*$/.test(segment))
  if (allCamel) return null
  if (specMatch && /^[A-Z]/.test(firstSegment) && siblingVueExists(absoluteDir, firstSegment)) {
    return null
  }
  return {
    kind: 'ts-module',
    path: relativePath,
    detail: `module '${fileName}' is not camelCase.ts`,
  }
}

function ancestorChain(relativePath: string): string[] {
  // features/admin/users/api.ts -> ['users']: the feature root itself is the
  // feature namespace (e.g. canvasFrameProbe under features/canvas stays
  // allowed); repeating a SUBDOMAIN ancestor is banned (WP-6).
  const parts = relativePath.split('/')
  if (parts[0] !== 'features') return []
  return parts.slice(2, -1)
}

function checkAntiStutter(relativePath: string, fileName: string): Violation | null {
  const ancestors = ancestorChain(relativePath)
  if (ancestors.length === 0) return null
  const stem = fileName.slice(0, -'.ts'.length)
  if (SPEC_SUFFIX_PATTERN.test(stem) || stem === 'index') return null
  const fileWords = splitWords(stem)
  for (const ancestor of ancestors) {
    const ancestorWords = splitWords(ancestor)
    if (fileWords.length < ancestorWords.length) continue
    if (!ancestorWords.every((word, index) => fileWords[index] === word)) continue
    if (fileWords.length === ancestorWords.length) continue
    const remainder = fileWords.slice(ancestorWords.length)
    const remainderHead = remainder[0] ?? ''
    if (
      remainder.length === 1 &&
      KIND_REMAINDER_ALLOWLIST.has(remainderHead[0]?.toUpperCase() + remainderHead.slice(1))
    ) {
      continue
    }
    return {
      kind: 'anti-stutter',
      path: relativePath,
      detail: `module '${stem}' repeats ancestor dir '${ancestor}'`,
    }
  }
  return null
}

function collectViolations(): Violation[] {
  const violations: Violation[] = []
  const visit = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const absolute = join(dir, entry)
      const relative = absolute.slice(SRC_ROOT.length + 1)
      const stat = statSync(absolute)
      if (stat.isDirectory()) {
        if (
          basename(dir) !== 'locales' &&
          !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(entry)
        ) {
          violations.push({
            kind: 'directory',
            path: relative,
            detail: `directory '${entry}' is not kebab-case`,
          })
        }
        if (entry === 'node_modules' || entry === 'generated-query') continue
        visit(absolute)
      } else if (entry.endsWith('.vue')) {
        const stem = basename(entry, '.vue')
        if (!/^[A-Z][A-Za-z0-9]*$/.test(stem)) {
          violations.push({
            kind: 'vue-sfc',
            path: relative,
            detail: `SFC '${entry}' is not PascalCase.vue`,
          })
        }
      } else if (entry.endsWith('.ts') && !entry.endsWith('.d.ts')) {
        if (absolute.includes('/generated-query/')) continue
        const moduleViolation = checkTsModule(relative, dir, entry)
        if (moduleViolation) violations.push(moduleViolation)
        const stutterViolation = checkAntiStutter(relative, entry)
        if (stutterViolation) violations.push(stutterViolation)
      }
    }
  }
  visit(SRC_ROOT)
  return violations.sort((a, b) => a.path.localeCompare(b.path))
}

function main() {
  const violations = collectViolations()
  const remainingAllowances = new Map(Object.entries(legacyAllowances))
  let failed = false

  for (const violation of violations) {
    if (remainingAllowances.has(violation.path)) {
      remainingAllowances.delete(violation.path)
      continue
    }
    failed = true
    console.error(`[naming-violation:${violation.kind}] ${violation.path} :: ${violation.detail}`)
  }

  for (const stale of [...remainingAllowances.keys()].sort()) {
    console.warn(`[naming-allowance:stale] ${stale} no longer violates; remove it from legacyAllowances`)
  }

  if (failed) {
    console.error('\nNaming governance check failed: new violations must be fixed, not baselined.')
    process.exit(1)
  }
  console.log(
    `Naming governance check passed: ${violations.length} ratcheted file(s), ${remainingAllowances.size} stale allowance(s).`,
  )
}

main()
