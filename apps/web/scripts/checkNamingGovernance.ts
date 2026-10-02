import { readdirSync, readFileSync, statSync } from 'node:fs'
import { basename, join } from 'node:path'

/**
 * Naming governance ratchet (issues #684, #692).
 *
 * Permanent rules (stay in CI; see the frontend DDD plan, filename governance):
 * 1. Directory names are kebab-case (BCP 47 locale codes directly under a
 *    `locales/` directory are exempt: they follow an external standard).
 * 2. Vue SFC files are PascalCase.vue.
 * 3. Non-component TypeScript modules are camelCase.ts, including web
 *    scripts (`scripts/*.ts`, plan section 20.1). Generated Orval
 *    output (`** /generated-query/**`) and ambient declarations (`*.d.ts`)
 *    are exempt. A PascalCase-prefixed spec is allowed only when it tests a
 *    colocated PascalCase SFC (e.g. `Foo.component.spec.ts` next to `Foo.vue`).
 * 4. Anti-stutter (features/ only): a module must not repeat any SUBDOMAIN
 *    ancestor directory name as a word prefix. The feature root itself is the
 *    feature namespace and stays allowed (e.g. canvasFrameProbe under
 *    features/canvas, Admin* inside admin/). Applies to TypeScript modules
 *    and Vue SFCs. A single trailing responsibility suffix (`Contract`,
 *    `Ref`, `Upload`, `Persistence` for TS; `Page`, `Layout`, `Dialog`,
 *    `Drawer`, `Card`, `Panel`, `Toolbar`, `Navigation`, `Form` for Vue) is
 *    allowed. `Pane` is the vocabulary-ratified Panel-equivalent tab-surface
 *    suffix (see `Pane` in `docs/architecture/frontend-vocabulary.yml`):
 *    the plan itself endorses the ancestor+suffix shape (`admin/users/
 *    UsersPage.vue`), and renaming the account tab panes to bare `Pane.vue`
 *    would collapse six distinct KeepAlive component names into one, changing
 *    caching behavior (plan section 11 forbids silent behavior changes).
 *    Excepted: `index.ts`, specs.
 * 5. Same-name directory/file pairs are forbidden (`controller/controller.ts`);
 *    use a responsibility name, merge the single-file directory up, or use
 *    `index.ts` only for a documented public boundary. Specs mirroring their
 *    subject are exempt.
 * 6. Generic names (`common`, `misc`, `utils`, `manager`, `service`,
 *    `handler`, `helpers`) are forbidden by default for files and directories. Scope
 *    exemptions: locale `common.ts` files (externally canonical locale keys)
 *    and the documented `keptGenericOwnership` narrow leaves below.
 * 7. Acronym normalization: camelCase module stems must not contain uppercase
 *    acronym runs (`API`, `SSE`, `URL`, `DTO`, `HTTP`, `IO`); use ordinary
 *    words (`api`, `sseClient`).
 * 8. Vue component responsibility: `*View.vue`, `*Manager.vue`,
 *    `*Service.vue` are forbidden; `Content.vue`/`Container.vue` require a
 *    documented ownership reason (`keptGenericOwnership`).
 * 9. Feature-local CSS/SCSS and image assets are kebab-case.
 * 10. Spec files must use `.(unit|contract|component|browser).spec.ts`.
 * 11. `types.ts` is allowed only in a leaf directory (no subdirectories) and
 *     only for multiple related type families; single-concept types use the
 *     concept name. Multi-family keeps are recorded in `keptMultiFamilyTypes`.
 * 12. Symbol alignment: `useX.ts` must export `useX` or a clearly related
 *     composable family recorded in `keptComposableFamilies`. `index.ts`
 *     coverage is rule 14. Deeper role-keyword checks (e.g. `resource.ts`
 *     must export `*Resource`) are intentionally out of scope: query-key
 *     helpers and DTOs colocate in `resource.ts` by established
 *     architecture (see shared/CONTEXT.md API Boundary and the contract
 *     inventory), so keyword matching would flag legitimate seams.
 * 13. Stale internal vocabulary: `User[A-Z]*` basenames under
 *    `features/user/`, `features/user-shell/`, or `features/workspace/` belong to the Workspace
 *     bounded context and are retired; use Workspace terminology. `User*` names
 *     in auth/shared refer to the user-identity concept, not the Workspace
 *     shell boundary, and are out of scope.
 * 14. `index.ts` is a barrel or documented module boundary only: after
 *     removing imports, re-exports (`export ... from`), `export type`, and
 *     one-line lazy `() => import()` wrappers, nothing executable may remain.
 *     Documented boundaries live in `keptBarrelBoundaries`.
 *
 * Migration machinery removed (issue #696): the phase-aware `migrationBaseline`
 * is gone with the final migration phase. Only permanent documented keeps
 * (`kept*` maps with owner + reason) and the empty `legacyAllowances`
 * ratchet remain.
 *
 * Ratchet: `legacyAllowances` tracks pre-existing violations outside the
 * documented keeps. Any violation NOT allowed fails the check; an allowance
 * whose file no longer violates prints a stale warning so it can be removed.
 */

const WEB_ROOT = join(import.meta.dirname, '..')
const SRC_ROOT = join(WEB_ROOT, 'src')
const SCRIPTS_ROOT = join(WEB_ROOT, 'scripts')

type ViolationKind =
  | 'directory'
  | 'vue-sfc'
  | 'ts-module'
  | 'anti-stutter'
  | 'same-name'
  | 'generic-name'
  | 'acronym'
  | 'vue-name'
  | 'style-asset'
  | 'spec-suffix'
  | 'types-placement'
  | 'symbol-alignment'
  | 'stale-prefix'
  | 'index-impl'

type Violation = {
  kind: ViolationKind
  path: string
  detail: string
}

const SPEC_SUFFIX_PATTERN = /\.(unit|contract|component|browser)\.spec$/
const SPEC_FILE_PATTERN = /\.(unit|contract|component|browser)\.spec\.ts$/
const KIND_REMAINDER_ALLOWLIST = new Set(['Contract', 'Ref', 'Upload', 'Persistence'])
// Plan section 20.6 responsibility suffixes (stable meanings), plus `Pane`
// ratified as the Panel-equivalent tab-surface suffix in
// docs/architecture/frontend-vocabulary.yml.
const VUE_RESPONSIBILITY_SUFFIXES = new Set([
  'Page',
  'Layout',
  'Dialog',
  'Drawer',
  'Card',
  'Panel',
  'Pane',
  'Toolbar',
  'Navigation',
  'Form',
])
const GENERIC_NAMES = new Set([
  'common',
  'misc',
  'utils',
  'manager',
  'service',
  'handler',
  'helpers',
])
const ACRONYM_RUNS = ['API', 'SSE', 'URL', 'DTO', 'HTTP', 'IO']
const VAGUE_VUE_SUFFIXES = ['View', 'Manager', 'Service']

const legacyAllowances: Record<string, string> = {}

/** Permanent documented keeps: src-relative dir path -> ownership reason. */
const keptGenericOwnership: Record<string, string> = {
  // Cross-pane shared leaf inside account presentation (panes, tab registry).
  'features/account/presentation/common': 'account-presentation: cross-pane shared leaf',
  // Cross-cutting client helpers (auth identity, errors, date, form, upload).
  'shared/utils': 'shared: cross-cutting client helpers (see shared/CONTEXT.md)',
  // Official shadcn-vue helper boundary (`components.json` standard utils location)
  'lib/utils.ts': 'shadcn: official standard helper entry (cn, cleanProps)',
  'lib/utils.unit.spec.ts': 'shadcn: test for official standard helper entry',
}

/** Permanent documented keeps: composable-family files (same responsibility family). */
const keptComposableFamilies: Record<string, string> = {
  'features/admin/settings/useSettingsFeedback.ts': 'admin: settings feedback composable family',
  'features/admin/monitor/application/useAdminMonitorFeedback.ts':
    'admin: task feedback composable family',
}

/** Permanent documented keeps: multi-family types.ts files. */
const keptMultiFamilyTypes: Record<string, string> = {
  'features/admin/payments/stripe-settings/types.ts':
    'admin: stripe configuration/summary/input families',
}

/** Permanent documented keeps: index.ts files that are module boundaries, not barrels. */
const keptBarrelBoundaries: Record<string, string> = {
  // The application router table: the documented route-URL boundary.
  'app/router/index.ts': 'app: route-URL boundary (see contract inventory)',
}

function splitWords(value: string): string[] {
  return value
    .replace(/[-_]+/g, ' ')
    .split(/(?=[A-Z])|\s+/)
    .filter(Boolean)
    .map((word) => word.toLowerCase())
}

function siblingVueExists(dir: string, prefix: string): boolean {
  try {
    return readdirSync(dir).some((file) => file === `${prefix}.vue`)
  } catch {
    return false
  }
}

function checkTsModule(
  relativePath: string,
  absoluteDir: string,
  fileName: string,
): Violation | null {
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
  // allowed); repeating a SUBDOMAIN ancestor is banned.
  const parts = relativePath.split('/')
  if (parts[0] !== 'features') return []
  return parts.slice(2, -1)
}

function checkAntiStutterStem(
  relativePath: string,
  stem: string,
  kind: ViolationKind,
): Violation | null {
  const ancestors = ancestorChain(relativePath)
  if (ancestors.length === 0) return null
  if (SPEC_SUFFIX_PATTERN.test(stem) || stem === 'index') return null
  const fileWords = splitWords(stem)
  for (const ancestor of ancestors) {
    const ancestorWords = splitWords(ancestor)
    if (fileWords.length < ancestorWords.length) continue
    if (!ancestorWords.every((word, index) => fileWords[index] === word)) continue
    if (fileWords.length === ancestorWords.length) continue
    const remainder = fileWords.slice(ancestorWords.length)
    if (remainder.length === 1) {
      const head = remainder[0] ?? ''
      const capitalized = head[0]?.toUpperCase() + head.slice(1)
      if (
        KIND_REMAINDER_ALLOWLIST.has(capitalized) ||
        VUE_RESPONSIBILITY_SUFFIXES.has(capitalized)
      ) {
        continue
      }
    }
    return {
      kind,
      path: relativePath,
      detail: `module '${stem}' repeats ancestor dir '${ancestor}'`,
    }
  }
  return null
}

function normalizeName(value: string): string {
  return value.toLowerCase().replace(/[-_]/g, '')
}

function checkSameName(
  relativePath: string,
  absoluteDir: string,
  fileName: string,
): Violation | null {
  if (fileName === 'index.ts' || fileName.endsWith('.spec.ts') || fileName.endsWith('.d.ts'))
    return null
  // `app/App.vue` is the canonical Vue application entry mounted by main.ts.
  if (relativePath === 'app/App.vue') return null
  // Official shadcn-vue compound component pattern (e.g. components/ui/button/Button.vue)
  if (relativePath.startsWith('components/ui/')) return null
  const parent = basename(absoluteDir)
  const stem = fileName.includes('.')
    ? (fileName.split('.')[0] ?? '')
    : fileName.replace(/\.(ts|vue)$/, '')
  if (stem && normalizeName(stem) === normalizeName(parent)) {
    return {
      kind: 'same-name',
      path: relativePath,
      detail: `file '${fileName}' repeats its directory name '${parent}'`,
    }
  }
  return null
}

function checkGenericName(
  relativePath: string,
  fileName: string,
  isDir: boolean,
): Violation | null {
  const name = isDir ? fileName : (fileName.split('.')[0] ?? '')
  const lowered = name.toLowerCase()
  // The list mixes forms (`utils`, `helpers` vs `service`, `manager`), so
  // match the trailing-s inflection too (`services/`, `managers/`).
  const singular = lowered.endsWith('s') ? lowered.slice(0, -1) : lowered
  if (!GENERIC_NAMES.has(lowered) && !GENERIC_NAMES.has(singular)) return null
  if (relativePath.includes('/locales/')) return null
  if (Object.hasOwn(keptGenericOwnership, relativePath)) return null
  return {
    kind: 'generic-name',
    path: relativePath,
    detail: `'${name}' hides ownership; use a specific responsibility name`,
  }
}

function checkAcronym(relativePath: string, fileName: string): Violation | null {
  if (fileName.endsWith('.spec.ts') || fileName.endsWith('.d.ts')) return null
  const stem = fileName.split('.')[0] ?? ''
  const hit = ACRONYM_RUNS.find((run) => stem.includes(run))
  if (hit) {
    return {
      kind: 'acronym',
      path: relativePath,
      detail: `module '${stem}' uses uppercase acronym '${hit}'; use ordinary camelCase words`,
    }
  }
  return null
}

function checkVueName(relativePath: string, fileName: string): Violation | null {
  const stem = basename(fileName, '.vue')
  if (stem === 'Content' || stem === 'Container') {
    if (!Object.hasOwn(keptGenericOwnership, relativePath)) {
      return {
        kind: 'vue-name',
        path: relativePath,
        detail: `'${stem}.vue' requires a documented ownership reason`,
      }
    }
    return null
  }
  if (VAGUE_VUE_SUFFIXES.some((suffix) => stem === suffix || stem.endsWith(suffix))) {
    return {
      kind: 'vue-name',
      path: relativePath,
      detail: `component '${stem}' uses a vague responsibility name`,
    }
  }
  return null
}

function checkStyleAsset(relativePath: string, fileName: string): Violation | null {
  if (!/^[a-z0-9]+(-[a-z0-9]+)*\.(css|scss|svg|png|jpg|jpeg|webp)$/.test(fileName)) {
    return {
      kind: 'style-asset',
      path: relativePath,
      detail: `style/asset '${fileName}' is not kebab-case`,
    }
  }
  return null
}

function checkSpecSuffix(relativePath: string, fileName: string): Violation | null {
  if (fileName.endsWith('.spec.ts') && !SPEC_FILE_PATTERN.test(fileName)) {
    return {
      kind: 'spec-suffix',
      path: relativePath,
      detail: `spec '${fileName}' must use .(unit|contract|component|browser).spec.ts`,
    }
  }
  return null
}

function checkTypesPlacement(
  relativePath: string,
  absoluteDir: string,
  fileName: string,
): Violation | null {
  if (fileName !== 'types.ts') return null
  if (Object.hasOwn(keptMultiFamilyTypes, relativePath)) return null
  try {
    const hasSubdir = readdirSync(absoluteDir).some((entry) => {
      try {
        return statSync(join(absoluteDir, entry)).isDirectory()
      } catch {
        return false
      }
    })
    if (hasSubdir) {
      return {
        kind: 'types-placement',
        path: relativePath,
        detail: `'types.ts' is allowed only in a narrow leaf directory`,
      }
    }
    const source = readFileSync(join(absoluteDir, fileName), 'utf8')
    return checkTypeFamilyCardinality(relativePath, source)
  } catch {
    return null
  }
}

function countExportedTypeFamilies(source: string): number {
  // A type-family keep is intentionally conservative: at least two exported
  // declarations must be present. Semantic relatedness stays documented in
  // keptMultiFamilyTypes because it cannot be inferred reliably from syntax.
  const exportedTypeDeclarations =
    /^\s*export\s+(?:declare\s+)?(?:type|interface|enum|class)\s+([A-Za-z0-9_]+)/gm
  return [...source.matchAll(exportedTypeDeclarations)].length
}

function checkTypeFamilyCardinality(relativePath: string, source: string): Violation | null {
  if (countExportedTypeFamilies(source) >= 2) return null
  return {
    kind: 'types-placement',
    path: relativePath,
    detail: `'types.ts' must contain multiple exported type families; rename single-concept types to a responsibility-specific module`,
  }
}

function checkStaleWorkspacePrefix(relativePath: string, fileName: string): Violation | null {
  if (fileName.endsWith('.spec.ts')) return null
  const underWorkspace =
    relativePath.startsWith('features/workspace/') ||
    relativePath.startsWith('features/user/') ||
    relativePath.startsWith('features/user-shell/')
  if (!underWorkspace) return null
  const stem = fileName.split('.')[0] ?? ''
  if (/^User[A-Z]/.test(stem) || stem === 'User') {
    return {
      kind: 'stale-prefix',
      path: relativePath,
      detail: `'${stem}' uses the retired User/User Shell vocabulary; use Workspace terminology`,
    }
  }
  return null
}

/**
 * Statement forms that belong in a barrel and carry no implementation.
 *
 * The repo formats without semicolons (`prettier.config.js` `semi: false`), so
 * none of these can lean on `;` as a terminator.
 *
 * The re-export pattern deliberately requires `*` or `{` right after `export`.
 * The previous `export\s+[^;]*?\sfrom` form could start at an `export const`
 * and run all the way to a *later* `from`, silently deleting the declaration it
 * was supposed to judge.
 */
const BARREL_SHAPED_LINES: RegExp[] = [
  // `export {…} from`, `export type {…} from`, `export * from`, `export * as ns from`
  /^export\s*(?:type\s*)?(?:\*(?:\s+as\s+[A-Za-z0-9_$]+)?|\{[^}]*\})\s*from\s*['"][^'"]+['"]\s*;?/gm,
  // `import {…} from`, `import x from`, `import 'side-effect'`
  /^\s*import\s[\s\S]*?from\s*['"][^'"]+['"]\s*;?/gm,
  /^\s*import\s*['"][^'"]+['"]\s*;?/gm,
  // `export type Foo = …` written inline (no `from`)
  /^export\s+type\s+[A-Za-z0-9_$]+(?:<[^=<>]*>)?\s*=[^;\n]*(?:\n\s+[^;\n]*)*/gm,
  // pure export lists of local bindings
  /^export\s*(?:type\s*)?\{[^}]*\}\s*;?/gm,
  // `export declare const/function …` has no runtime body but is still a
  // declaration, not a seam.
]

/** The single lazy wrapper a barrel may declare: `export const loadX = () => import('./x')`. */
const LAZY_IMPORT_WRAPPER =
  /export\s+const\s+[A-Za-z0-9_]+\s*=\s*(?:\([^)]*\)|[A-Za-z0-9_]+)\s*=>\s*import\s*\([^)]*\)\s*;?/g

/**
 * Rule 14. `index.ts` is a barrel or a documented module boundary only.
 *
 * The old check missed every `export const/let/var` binding that survived the
 * lazy-wrapper strip, because the residual test only looked for a bare
 * `const|let|var|function` at the start of a line — `export const x = (a) => …`
 * starts with `export`, so a whole barrel's worth of business logic passed as
 * long as it was declared with `export`.
 */
function checkIndexBarrelBody(relativePath: string, source: string): Violation | null {
  const withoutComments = source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
  const stripped = BARREL_SHAPED_LINES.reduce(
    (accumulated, pattern) => accumulated.replace(pattern, ''),
    withoutComments,
  ).replace(LAZY_IMPORT_WRAPPER, '')

  const barrelImpl = (): Violation => ({
    kind: 'index-impl',
    path: relativePath,
    detail: `'index.ts' hides business implementation; keep barrels re-export-only`,
  })

  if (/:\s*(Vue|Component)\b/.test(stripped)) return barrelImpl()

  // Any surviving `export`ed declaration is implementation, whether it is a
  // function, a class, an enum, or a `const`/`let`/`var` binding — including
  // arrow-function consts, which the previous residual test could not see.
  if (
    /^export\s+(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?(?:function\*?|class|const|let|var|enum)\b/m.test(
      stripped,
    )
  )
    return barrelImpl()

  if (/(^|\n)\s*(const|let|var|function)\s+[A-Za-z0-9_]/.test(stripped.trim())) return barrelImpl()

  return null
}

function checkIndexBarrel(
  relativePath: string,
  absolutePath: string,
  fileName: string,
): Violation | null {
  if (fileName !== 'index.ts') return null
  if (Object.hasOwn(keptBarrelBoundaries, relativePath)) return null
  try {
    return checkIndexBarrelBody(relativePath, readFileSync(absolutePath, 'utf8'))
  } catch {
    return null
  }
}

/**
 * Rule 12. Collect the symbols a `useX.ts` module actually exports.
 *
 * The old pattern was `/^export\s+(?:function|const|type|interface|class)\s+(\w+)/`,
 * so `export async function useFoo` and `export default function useFoo` matched
 * nothing at all: `detected` came back empty and the guard reported the file
 * as if it had no exports, which reads as "aligned" rather than "unknown".
 */
const EXPORTED_SYMBOL =
  /^export\s+(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?(?:function\*?|const|let|var|type|interface|class|enum)\s+([A-Za-z0-9_$]+)/gm

function collectExportedSymbols(source: string): Set<string> {
  const exported = new Set<string>()
  for (const match of source.matchAll(EXPORTED_SYMBOL)) exported.add(match[1] ?? '')
  return exported
}

function checkSymbolAlignment(
  relativePath: string,
  absolutePath: string,
  fileName: string,
): Violation | null {
  let source: string
  try {
    source = readFileSync(absolutePath, 'utf8')
  } catch {
    return null
  }
  return checkSymbolAlignmentSource(relativePath, fileName, source)
}

/** Pure half of rule 12, so the self-test can pin the bypass shapes. */
function checkSymbolAlignmentSource(
  relativePath: string,
  fileName: string,
  source: string,
): Violation | null {
  if (!/^use[A-Z]/.test(fileName) || fileName.endsWith('.spec.ts')) return null
  if (Object.hasOwn(keptComposableFamilies, relativePath)) return null
  const stem = fileName.split('.')[0] ?? ''
  if (collectExportedSymbols(source).has(stem)) return null
  return {
    kind: 'symbol-alignment',
    path: relativePath,
    detail: `'${fileName}' must export '${stem}' or a documented composable family`,
  }
}

/** Rule 14 on an in-memory source, so the self-test can pin the bypass shapes. */
function checkIndexBarrelSource(
  relativePath: string,
  fileName: string,
  source: string,
): Violation | null {
  if (fileName !== 'index.ts') return null
  if (Object.hasOwn(keptBarrelBoundaries, relativePath)) return null
  return checkIndexBarrelBody(relativePath, source)
}

function collectViolations(): Violation[] {
  const violations: Violation[] = []
  const visit = (dir: string, root: string) => {
    for (const entry of readdirSync(dir)) {
      const absolute = join(dir, entry)
      const underRoot = absolute.slice(root.length + 1)
      const relative = root === SCRIPTS_ROOT ? `scripts/${underRoot}` : underRoot
      const stat = statSync(absolute)
      if (stat.isDirectory()) {
        if (root === SRC_ROOT) {
          if (basename(dir) !== 'locales' && !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(entry)) {
            violations.push({
              kind: 'directory',
              path: relative,
              detail: `directory '${entry}' is not kebab-case`,
            })
          }
          const generic = checkGenericName(relative, entry, true)
          if (generic) violations.push(generic)
        }
        if (entry === 'node_modules' || entry === 'generated-query') continue
        visit(absolute, root)
      } else if (entry.endsWith('.vue')) {
        const stem = basename(entry, '.vue')
        if (!/^[A-Z][A-Za-z0-9]*$/.test(stem)) {
          violations.push({
            kind: 'vue-sfc',
            path: relative,
            detail: `SFC '${entry}' is not PascalCase.vue`,
          })
        } else {
          const vague = checkVueName(relative, entry)
          if (vague) violations.push(vague)
          const stutter = checkAntiStutterStem(relative, stem, 'anti-stutter')
          if (stutter) violations.push(stutter)
          const same = checkSameName(relative, dir, entry)
          if (same) violations.push(same)
          const acronym = checkAcronym(relative, entry)
          if (acronym) violations.push(acronym)
          const stale = checkStaleWorkspacePrefix(relative, entry)
          if (stale) violations.push(stale)
        }
      } else if (entry.endsWith('.ts') && !entry.endsWith('.d.ts')) {
        if (absolute.includes('/generated-query/')) continue
        if (root === SCRIPTS_ROOT) {
          // Web scripts follow the same camelCase module rules (plan 20.1).
          const moduleViolation = checkTsModule(relative, dir, entry)
          if (moduleViolation) violations.push(moduleViolation)
          const acronym = checkAcronym(relative, entry)
          if (acronym) violations.push(acronym)
          const suffix = checkSpecSuffix(relative, entry)
          if (suffix) violations.push(suffix)
          continue
        }
        const moduleViolation = checkTsModule(relative, dir, entry)
        if (moduleViolation) violations.push(moduleViolation)
        const stutterViolation = checkAntiStutterStem(
          relative,
          entry.slice(0, -'.ts'.length),
          'anti-stutter',
        )
        if (stutterViolation) violations.push(stutterViolation)
        const same = checkSameName(relative, dir, entry)
        if (same) violations.push(same)
        const generic = checkGenericName(relative, entry, false)
        if (generic) violations.push(generic)
        const acronym = checkAcronym(relative, entry)
        if (acronym) violations.push(acronym)
        const suffix = checkSpecSuffix(relative, entry)
        if (suffix) violations.push(suffix)
        const placement = checkTypesPlacement(relative, dir, entry)
        if (placement) violations.push(placement)
        const alignment = checkSymbolAlignment(relative, absolute, entry)
        if (alignment) violations.push(alignment)
        const stale = checkStaleWorkspacePrefix(relative, entry)
        if (stale) violations.push(stale)
        const barrel = checkIndexBarrel(relative, absolute, entry)
        if (barrel) violations.push(barrel)
      } else if (/\.(css|scss|svg|png|jpg|jpeg|webp)$/.test(entry)) {
        const style = checkStyleAsset(relative, entry)
        if (style) violations.push(style)
      }
    }
  }
  visit(SRC_ROOT, SRC_ROOT)
  visit(SCRIPTS_ROOT, SCRIPTS_ROOT)
  return violations.sort((a, b) => a.path.localeCompare(b.path))
}

function runSelfTest(): void {
  const cases: Array<[string, unknown, unknown]> = [
    ['kebab-dir-ok', /^[a-z0-9]+(-[a-z0-9]+)*$/.test('stripe-settings'), true],
    ['kebab-dir-bad', /^[a-z0-9]+(-[a-z0-9]+)*$/.test('stripeSettings'), false],
    ['spec-suffix-ok', SPEC_FILE_PATTERN.test('resource.unit.spec.ts'), true],
    ['spec-suffix-bare', SPEC_FILE_PATTERN.test('resource.spec.ts'), false],
    [
      'vue-suffix-remainder-allowed',
      checkAntiStutterStem(
        'features/account/presentation/security/SecurityPane.vue',
        'SecurityPane',
        'anti-stutter',
      ),
      null,
    ],
    [
      'vue-prefix-flagged',
      checkAntiStutterStem(
        'features/canvas/tools/retouch/components/Chrome.vue',
        'RetouchChrome',
        'anti-stutter',
      )?.kind,
      'anti-stutter',
    ],
    [
      'same-name-forbidden',
      checkSameName('features/x/controller/controller.ts', 'features/x/controller', 'controller.ts')
        ?.kind,
      'same-name',
    ],
    [
      'same-name-index-exempt',
      checkSameName('features/x/controller/index.ts', 'features/x/controller', 'index.ts'),
      null,
    ],
    [
      'generic-utils',
      checkGenericName('features/x/utils.ts', 'utils.ts', false)?.kind,
      'generic-name',
    ],
    [
      'generic-locale-exempt',
      checkGenericName('shared/i18n/locales/en/common.ts', 'common.ts', false),
      null,
    ],
    ['acronym-api', checkAcronym('features/x/providerAPI.ts', 'providerAPI.ts')?.kind, 'acronym'],
    ['acronym-ok', checkAcronym('features/x/sseClient.ts', 'sseClient.ts'), null],
    ['vague-view', checkVueName('features/x/FooView.vue', 'FooView.vue')?.kind, 'vue-name'],
    [
      'content-needs-reason',
      checkVueName('features/x/Content.vue', 'Content.vue')?.kind,
      'vue-name',
    ],
    [
      'style-kebab-ok',
      checkStyleAsset('shared/motion/dialog-motion.css', 'dialog-motion.css'),
      null,
    ],
    [
      'style-camel-bad',
      checkStyleAsset('shared/motion/dialogMotion.css', 'dialogMotion.css')?.kind,
      'style-asset',
    ],
    [
      'kept-generic-dir',
      checkGenericName('features/account/presentation/common', 'common', true),
      null,
    ],
    [
      'generic-flagged',
      checkGenericName('features/x/utils.ts', 'utils.ts', false)?.kind,
      'generic-name',
    ],
    [
      'stale-user-prefix',
      checkStaleWorkspacePrefix('features/user/layout/UserLayout.vue', 'UserLayout.vue')?.kind,
      'stale-prefix',
    ],
    [
      'stale-workspace-user-prefix',
      checkStaleWorkspacePrefix('features/workspace/layout/UserLayout.vue', 'UserLayout.vue')?.kind,
      'stale-prefix',
    ],
    [
      'user-identity-out-of-scope',
      checkStaleWorkspacePrefix(
        'features/auth/components/UserAuthModals.vue',
        'UserAuthModals.vue',
      ),
      null,
    ],
    [
      'types-multi-family-ok',
      checkTypeFamilyCardinality(
        'features/example/types.ts',
        'export type Alpha = string\nexport interface Beta { value: number }',
      ),
      null,
    ],
    [
      'types-single-family-rejected',
      checkTypeFamilyCardinality(
        'features/example/types.ts',
        'export type ResolvedTaskImageResult = { url: string }',
      )?.kind,
      'types-placement',
    ],
    // --- rule 12: symbol-alignment -----------------------------------
    // The old export regex was `/^export\s+(?:function|const|type|interface|class)\s+(\w+)/`,
    // so `export async function useThing` and `export default function useThing`
    // contributed *nothing* to `exported` — the file looked like it had no
    // exports at all, i.e. a correctly aligned composable was reported as
    // misaligned for the wrong reason.
    [
      'symbol-alignment-async-function',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export async function useThing() {\n  return 1\n}\n',
      ),
      null,
    ],
    [
      'symbol-alignment-default-function',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export default function useThing() {\n  return 1\n}\n',
      ),
      null,
    ],
    [
      'symbol-alignment-plain-function',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export function useThing() {\n  return 1\n}\n',
      ),
      null,
    ],
    [
      'symbol-alignment-async-mismatch',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export async function useOther() {\n  return 1\n}\n',
      )?.kind,
      'symbol-alignment',
    ],
    [
      'symbol-alignment-default-mismatch',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export default function useOther() {\n  return 1\n}\n',
      )?.kind,
      'symbol-alignment',
    ],
    [
      'symbol-alignment-detects-async-and-default',
      [
        ...collectExportedSymbols('export async function useThing() {}'),
        ...collectExportedSymbols('export default function useThing() {}'),
      ],
      ['useThing', 'useThing'],
    ],
    // --- rule 14: index-impl -----------------------------------------
    // The old residual test only looked for a bare `const|let|var|function`
    // at the start of a line, so `export const x = (a) => …` — the exact shape
    // the rule exists to forbid — sailed through.
    [
      'index-impl-arrow-const',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        'export const total = (a, b) => a + b\n',
      )?.kind,
      'index-impl',
    ],
    [
      'index-impl-plain-const',
      checkIndexBarrelSource('features/example/index.ts', 'index.ts', 'export const LIMIT = 3\n')
        ?.kind,
      'index-impl',
    ],
    [
      'index-impl-async-function',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        'export async function loadThing() {\n  return 1\n}\n',
      )?.kind,
      'index-impl',
    ],
    [
      'index-impl-default-export',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        'export default function loadThing() {\n  return 1\n}\n',
      )?.kind,
      'index-impl',
    ],
    [
      'index-impl-not-swallowed-by-a-later-from',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        "export const total = (a, b) => a + b\n\nexport { other } from './other'\n",
      )?.kind,
      'index-impl',
    ],
    [
      'index-impl-local-const',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        'const total = (a, b) => a + b\n',
      )?.kind,
      'index-impl',
    ],
    [
      'index-barrel-reexports-ok',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        "export { a, type B } from './a'\n\nexport * from './b'\n\nexport {\n  c,\n  type D,\n} from './c'\n",
      ),
      null,
    ],
    [
      'index-barrel-lazy-wrapper-ok',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        "export const loadThing = () => import('./thing')\n",
      ),
      null,
    ],
    [
      'index-barrel-type-only-ok',
      checkIndexBarrelSource(
        'features/example/index.ts',
        'index.ts',
        'export type Thing = {\n  id: string\n}\n',
      ),
      null,
    ],
    [
      'index-barrel-kept-boundary',
      checkIndexBarrelSource('app/router/index.ts', 'index.ts', 'export const routes = build()\n'),
      null,
    ],
  ]
  let failed = 0
  for (const [name, actual, expected] of cases) {
    const actualValue = JSON.stringify(actual)
    if (actualValue !== JSON.stringify(expected)) {
      failed += 1
      console.error(
        `[naming-self-test] ${name}: expected ${JSON.stringify(expected)}, got ${actualValue}`,
      )
    }
  }
  if (failed > 0) {
    console.error(`\nNaming governance self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`Naming governance self-test passed: ${cases.length} case(s).`)
}

function main() {
  runSelfTest()
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
    console.warn(
      `[naming-allowance:stale] ${stale} no longer violates; remove it from legacyAllowances`,
    )
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
