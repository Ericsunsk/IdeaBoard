import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { basename, join } from 'node:path'
import ts from 'typescript'

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
 *    ADR-0013 tool zones (`features/<context>/tools/<tool>/session|ui/`):
 *    `session/` files are exempt (tool-prefixed orchestration modules such as
 *    `session/retouchViewModel.ts` are the documented session vocabulary, not
 *    stutter); a Vue SFC directly under `ui/<control>/` may share its control
 *    directory's stem (`ui/curves/CurvesControl.vue`). Other ancestors and
 *    non-SFC modules under `ui/<control>/` stay governed.
 * 5. Same-name directory/file pairs are forbidden (`controller/controller.ts`);
 *    use a responsibility name, merge the single-file directory up, or use
 *    `index.ts` only for a documented public boundary. Specs mirroring their
 *    subject are exempt. ADR-0013 control-colocated SFCs are exempt: one
 *    directory per control, so the control entry may carry the control name
 *    (`ui/chrome/Chrome.vue`).
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
 *     architecture (see core/CONTEXT.md API Boundary and the contract
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
 * 15. Vague responsibility suffixes (`Manager`, `Helper`, `Util`, `Data`) are
 *     forbidden as file stem suffixes for Vue SFCs and TypeScript modules.
 *     Specs mirroring their subject are exempt; pre-existing documented keeps
 *     live in `keptRoleSuffixOwnership`. Any new `*Manager/*Helper/*Util/*Data`
 *     file fails the check.
 * 16. API vocabulary `*_url` (warn-only, never fails): new image/avatar URL
 *     fields must use the `*_url` suffix (plan §3.6). Pre-existing `avatar`
 *     names are grandfathered in `keptLegacyUrlFields` (recorded AIP-140
 *     deviation); named API/resource declarations or explicit URL annotations
 *     establish URL semantics. Ordinary display strings and consumers never warn.
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
  | 'role-suffix'

type Violation = {
  kind: ViolationKind
  path: string
  detail: string
}

const SPEC_SUFFIX_PATTERN = /\.(unit|contract|component|browser)\.spec$/
const SPEC_FILE_PATTERN = /\.(unit|contract|component|browser)\.spec\.ts$/
const KIND_REMAINDER_ALLOWLIST = new Set([
  'Contract',
  'Ref',
  'Upload',
  'Persistence',
  // Narrow kind suffixes for the admin shared regroup renames: `ui/styling/uiClasses.ts`
  // repeats ancestor `ui`, `ui/table/tableColumns.ts` repeats ancestor `table`.
  // `Controller` covers the gallery-level controller (`ui/gallery/controllers/galleryController.ts`
  // repeats ancestor `gallery`): the `controllers/` group holds sibling controller modules, so the
  // gallery entry carries its subdomain plus the responsibility kind. Single-word helper-kind
  // remainders only; anything else still flags.
  'Classes',
  'Columns',
  'Controller',
])
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
const ROLE_SUFFIXES = ['Manager', 'Helper', 'Util', 'Data']

const legacyAllowances: Record<string, string> = {
  // Accepted shared regroup variance (owner ruling 2026-10-11): session keeps
  // per-scope subdirs, ui keeps group dirs, core stays flat. These stems repeat
  // their immediate group dir by design; renaming would churn the accepted tree.
  'features/admin/shared/session/brand/brandLogoResource.ts':
    'admin: brand scope resource repeats brand group dir by design',
  'features/admin/shared/session/brand/brandLogos.ts':
    'admin: brand vocabulary repeats brand group dir by design',
  'features/admin/shared/ui/chart/chart.ts':
    'admin: chart primitive repeats chart group dir by design',
  'features/board/ui/gallery/range/range.ts':
    'gallery: range primitive repeats range group dir by design',
  'features/admin/users/session/sessionCommands.ts':
    'admin: session commands repeat session zone by design',
}

/** Permanent documented keeps: src-relative dir path -> ownership reason. */
const keptGenericOwnership: Record<string, string> = {
  // Cross-cutting client helpers (auth identity, errors, date, form, upload).
  'core/utils': 'core: cross-cutting client helpers (see core/CONTEXT.md)',
  // Official shadcn-vue helper boundary (`components.json` standard utils location)
  'lib/utils.ts': 'shadcn: official standard helper entry (cn, cleanProps)',
  'lib/utils.unit.spec.ts': 'shadcn: test for official standard helper entry',
}

/** Permanent documented keeps: pre-existing `*Data` modules (rule 15). */
const keptRoleSuffixOwnership: Record<string, string> = {
  'features/admin/statistics/session/useUsageStatisticsData.ts':
    'admin: usage statistics data composable (pre-existing; new *Data modules banned)',
  'features/admin/models/core/catalogData.ts':
    'admin: model catalog reference data (pre-existing; new *Data modules banned)',
}

/** Permanent documented keeps: pre-existing `image/avatar` URL fields (rule 16, warn-only). */
const keptLegacyUrlFields = new Set([
  'api/http/sessionUserType.ts:User.avatar',
  'api/media/upload.ts:UploadAvatarMetadata.avatar',
  'features/workspace/selection/workspaceResource.ts:PrincipalSummary.avatar',
  'features/admin/users/session/resource.ts:AdminUserListItem.avatar',
  'test/factories/admin.ts:AdminListItemOverrides.avatar',
  'test/factories/admin.ts:ControlUserPayloadOverrides.avatar',
])
const keptComposableFamilies: Record<string, string> = {
  'features/admin/settings/session/useSettingsFeedback.ts':
    'admin: settings feedback composable family',
  'features/admin/monitor/session/useAdminMonitorFeedback.ts':
    'admin: task feedback composable family',
}

/** Permanent documented keeps: multi-family types.ts files. */
const keptMultiFamilyTypes: Record<string, string> = {
  'features/admin/settings/session/payment/types.ts':
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

/**
 * ADR-0013 tool zones: `features/<context>/tools/<tool>/session|ui/`.
 *
 * `session/` owns stateful orchestration, so tool-prefixed session modules
 * (`session/retouchViewModel.ts`, `useRetouchSession.ts`) are the documented
 * session vocabulary, not stutter. `ui/<control>/` colocates one control's
 * SFC with its helpers, so a Vue SFC directly under it may share the control
 * directory's stem (`ui/curves/CurvesControl.vue`).
 */
function isToolSession(relativePath: string): boolean {
  return /^features\/[^/]+\/tools\/[^/]+\/session(?:\/|$)/.test(relativePath)
}

/**
 * Canvas core graph domain keep: the canvasGraph* -> graph* rename left 16
 * `graph*` modules repeating their `graph/` group dir by design (graphStore,
 * graphMutation, ...). Pinned as an explicit file list, not a directory
 * regex, so a future `graph/graph*.ts` stutter is still flagged.
 * `groupGeometry.ts`, `libraryImportState.ts`, spec files and `index.ts` need
 * no exemption: they do not repeat an ancestor stem.
 */
const CANVAS_CORE_GRAPH_STUTTER_KEEP = new Set([
  'features/canvas/core/graph/graphClone.ts',
  'features/canvas/core/graph/graphGeometry.ts',
  'features/canvas/core/graph/graphHistoryProjection.ts',
  'features/canvas/core/graph/graphMutation.ts',
  'features/canvas/core/graph/graphMutationIntents.ts',
  'features/canvas/core/graph/graphPatchAlgebra.ts',
  'features/canvas/core/graph/graphRecordEquality.ts',
  'features/canvas/core/graph/graphStore.ts',
  'features/canvas/core/graph/graphStoreIds.ts',
  'features/canvas/core/graph/graphStoreIndexes.ts',
  'features/canvas/core/graph/graphStoreState.ts',
  'features/canvas/core/graph/graphStoreTelemetry.ts',
  'features/canvas/core/graph/graphStoreTypes.ts',
  'features/canvas/core/graph/graphStoreWriter.ts',
  'features/canvas/core/graph/graphTransactionPort.ts',
  'features/canvas/core/graph/graphValueEquality.ts',
])

/** Canvas core graph domain modules use the graph domain prefix (e.g. graphStore, graphMutation). */
function isCanvasCoreGraph(relativePath: string): boolean {
  return CANVAS_CORE_GRAPH_STUTTER_KEEP.has(relativePath)
}

/**
 * Stale self-check for the canvas graph keep: every entry must still exist on
 * disk. A rename/delete leaves a dead entry with no violation to consume it
 * (unlike `legacyAllowances`, which warns on unused paths), so missing files
 * fail the check with a cleanup prompt.
 */
function collectCanvasKeepStale(
  exists: (relativePath: string) => boolean = (relativePath) => existsSync(join(SRC_ROOT, relativePath)),
): string[] {
  return [...CANVAS_CORE_GRAPH_STUTTER_KEEP].filter((relativePath) => !exists(relativePath)).sort()
}

/**
 * Generic keep staleness (existence + still-hit double assertion).
 *
 * Every `kept*` entry must (1) still exist on disk and (2) still need its
 * keep: without the keep entry the file would still flag under the same
 * rule. A rename/delete fails on (1); a fixed file fails on (2) with a
 * cleanup prompt. Predicates below bypass their own keep maps so the
 * "still hits" half cannot self-exempt.
 */
function genericKeepStillHits(relativePath: string): boolean {
  if (relativePath.includes('/locales/')) return false
  const fileName = relativePath.split('/').at(-1) ?? ''
  // keptGenericOwnership holds both directories (`core/utils`) and files
  // (`lib/utils.ts`): directories judge by their own name, files by stem.
  const name = fileName.includes('.') ? ((fileName.split('.')[0] ?? '').toLowerCase()) : fileName.toLowerCase()
  const singular = name.endsWith('s') ? name.slice(0, -1) : name
  return GENERIC_NAMES.has(name) || GENERIC_NAMES.has(singular)
}

function roleSuffixKeepStillHits(relativePath: string): boolean {
  const fileName = relativePath.split('/').at(-1) ?? ''
  if (fileName.endsWith('.spec.ts') || fileName.endsWith('.d.ts')) return false
  const stem = fileName.endsWith('.vue')
    ? basename(fileName, '.vue')
    : fileName.slice(0, -'.ts'.length)
  return ROLE_SUFFIXES.some((suffix) => stem.endsWith(suffix))
}

function composableKeepStillHits(source: string, fileName: string): boolean {
  if (fileName.endsWith('.shim.ts')) return false
  if (!/^use[A-Z]/.test(fileName) || fileName.endsWith('.spec.ts')) return false
  if (source.includes('compatibility shim') && /export\s+\*\s+from/.test(source)) return false
  const stem = fileName.split('.')[0] ?? ''
  return !collectExportedSymbols(source).has(stem)
}

function multiFamilyKeepStillHits(source: string): boolean {
  return countExportedTypeFamilies(source) >= 2
}

function barrelKeepStillHits(source: string, relativePath: string): boolean {
  return checkIndexBarrelBody(relativePath, source) !== null
}

function legacyUrlKeepStillHits(source: string, identity: string): boolean {
  const parts = identity.split('.')
  const owner = parts[0] ?? ''
  const leaf = parts.at(-1) ?? ''
  return source.includes(owner) && source.includes(leaf)
}

function collectKeepStale(
  exists: (relativePath: string) => boolean = (relativePath) =>
    existsSync(join(SRC_ROOT, relativePath)),
  read: (relativePath: string) => string | null = (relativePath) => {
    try {
      return readFileSync(join(SRC_ROOT, relativePath), 'utf8')
    } catch {
      return null
    }
  },
): string[] {
  const stale: string[] = []
  for (const relativePath of Object.keys(keptGenericOwnership)) {
    if (!exists(relativePath)) {
      stale.push(`keptGenericOwnership:${relativePath} (missing)`)
      continue
    }
    if (!genericKeepStillHits(relativePath)) {
      stale.push(`keptGenericOwnership:${relativePath} (unneeded)`)
    }
  }
  for (const relativePath of Object.keys(keptRoleSuffixOwnership)) {
    if (!exists(relativePath)) {
      stale.push(`keptRoleSuffixOwnership:${relativePath} (missing)`)
      continue
    }
    if (!roleSuffixKeepStillHits(relativePath)) {
      stale.push(`keptRoleSuffixOwnership:${relativePath} (unneeded)`)
    }
  }
  for (const relativePath of Object.keys(keptComposableFamilies)) {
    if (!exists(relativePath)) {
      stale.push(`keptComposableFamilies:${relativePath} (missing)`)
      continue
    }
    const fileName = relativePath.split('/').at(-1) ?? ''
    const source = read(relativePath)
    if (source === null || !composableKeepStillHits(source, fileName)) {
      stale.push(
        `keptComposableFamilies:${relativePath} (${source === null ? 'missing' : 'unneeded'})`,
      )
    }
  }
  for (const relativePath of Object.keys(keptMultiFamilyTypes)) {
    if (!exists(relativePath)) {
      stale.push(`keptMultiFamilyTypes:${relativePath} (missing)`)
      continue
    }
    const source = read(relativePath)
    if (source === null || !multiFamilyKeepStillHits(source)) {
      stale.push(
        `keptMultiFamilyTypes:${relativePath} (${source === null ? 'missing' : 'unneeded'})`,
      )
    }
  }
  for (const relativePath of Object.keys(keptBarrelBoundaries)) {
    if (!exists(relativePath)) {
      stale.push(`keptBarrelBoundaries:${relativePath} (missing)`)
      continue
    }
    const source = read(relativePath)
    if (source === null || !barrelKeepStillHits(source, relativePath)) {
      stale.push(
        `keptBarrelBoundaries:${relativePath} (${source === null ? 'missing' : 'unneeded'})`,
      )
    }
  }
  for (const entry of keptLegacyUrlFields) {
    const separator = entry.indexOf(':')
    const relativePath = separator === -1 ? entry : entry.slice(0, separator)
    const identity = separator === -1 ? '' : entry.slice(separator + 1)
    if (!exists(relativePath)) {
      stale.push(`keptLegacyUrlFields:${entry} (missing)`)
      continue
    }
    const source = read(relativePath)
    if (source === null || !legacyUrlKeepStillHits(source, identity)) {
      stale.push(`keptLegacyUrlFields:${entry} (${source === null ? 'missing' : 'unneeded'})`)
    }
  }
  return stale.sort()
}

/** ADR-0013 control-colocated SFC: a `.vue` file directly under `ui/<control>/`. */
function isControlColocatedSfc(relativePath: string, fileName: string): boolean {
  if (!fileName.endsWith('.vue')) return false
  return /^features\/[^/]+\/tools\/[^/]+\/ui\/[^/]+\/[^/]+$/.test(relativePath)
}

function checkAntiStutterStem(
  relativePath: string,
  stem: string,
  kind: ViolationKind,
  options?: { controlColocatedSfc?: boolean },
): Violation | null {
  // ADR-0013: session-layer files are not judged for anti-stutter.
  if (isToolSession(relativePath)) return null
  if (isCanvasCoreGraph(relativePath)) return null
  const ancestors = ancestorChain(relativePath)
  if (ancestors.length === 0) return null
  if (SPEC_SUFFIX_PATTERN.test(stem) || stem === 'index') return null
  const parent = relativePath.split('/').slice(-2, -1)[0] ?? ''
  const fileWords = splitWords(stem)
  for (const ancestor of ancestors) {
    // ADR-0013: a control-colocated SFC may repeat its own control directory.
    // Any other ancestor (tool scope, `ui`, sibling zones) stays governed.
    if (options?.controlColocatedSfc === true && ancestor === parent) continue
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
    // ADR-0013: a control-colocated SFC may carry its control directory's name
    // (`ui/chrome/Chrome.vue`); deeper nesting and non-SFC modules stay governed.
    if (isControlColocatedSfc(relativePath, fileName)) return null
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

/**
 * Rule 15. Vague responsibility suffixes (`Manager`, `Helper`, `Util`,
 * `Data`) hide ownership: use a specific responsibility name. Specs mirroring
 * their subject are exempt; pre-existing modules live in
 * `keptRoleSuffixOwnership`. The match is a case-sensitive stem suffix, so
 * `mediaAssetMetadata.ts` (lowercase `data`) and `imageDataUtils.ts`
 * (`Utils`, not `Util`) stay out of scope.
 */
function checkRoleSuffix(relativePath: string, fileName: string): Violation | null {
  if (fileName.endsWith('.spec.ts') || fileName.endsWith('.d.ts')) return null
  if (Object.hasOwn(keptRoleSuffixOwnership, relativePath)) return null
  const stem = fileName.endsWith('.vue')
    ? basename(fileName, '.vue')
    : fileName.slice(0, -'.ts'.length)
  const hit = ROLE_SUFFIXES.find((suffix) => stem.endsWith(suffix))
  if (hit) {
    return {
      kind: 'role-suffix',
      path: relativePath,
      detail: `file '${fileName}' uses vague responsibility suffix '${hit}'; use a specific responsibility name`,
    }
  }
  return null
}

/**
 * Rule 16 (warn-only). New image/avatar URL fields must use the `*_url`
 * suffix. Parse named API/resource shapes and explicitly documented URL fields;
 * an arbitrary display string named `image` is not necessarily a URL. Existing
 * exceptions freeze the declaration/member, never the entire file.
 */
function isUrlStringType(type: ts.TypeNode | undefined): boolean {
  if (!type) return false
  if (type.kind === ts.SyntaxKind.StringKeyword) return true
  if (ts.isParenthesizedTypeNode(type)) return isUrlStringType(type.type)
  if (ts.isArrayTypeNode(type)) return isUrlStringType(type.elementType)
  if (ts.isTypeReferenceNode(type) && /^(?:ReadonlyArray|Array)$/.test(type.typeName.getText())) {
    return isUrlStringType(type.typeArguments?.[0])
  }
  if (ts.isUnionTypeNode(type)) {
    const concrete = type.types.filter(
      (member) =>
        member.kind !== ts.SyntaxKind.UndefinedKeyword &&
        !(ts.isLiteralTypeNode(member) && member.literal.kind === ts.SyntaxKind.NullKeyword),
    )
    return concrete.length > 0 && concrete.every(isUrlStringType)
  }
  return false
}

function collectUrlSuffixWarnings(
  relativePath: string,
  fileName: string,
  source: string,
): Array<{ path: string; detail: string }> {
  if (fileName.endsWith('.spec.ts') || fileName.endsWith('.d.ts')) return []
  if (relativePath.includes('/generated-query/')) return []
  const parsed = ts.createSourceFile(relativePath, source, ts.ScriptTarget.Latest, true)
  const apiBoundary =
    /^(?:shared\/api\/|api\/|test\/factories\/)/.test(relativePath) ||
    /(?:^|\/)(?:resource|[A-Za-z0-9]*Resource|[A-Za-z0-9]*Dto|[A-Za-z0-9]*DTO)\.ts$/.test(
      relativePath,
    )
  const warnings: Array<{ path: string; detail: string }> = []
  const inspectDeclaration = (declaration: ts.InterfaceDeclaration | ts.TypeAliasDeclaration) => {
    const owner = declaration.name.text
    const declaredBoundary =
      apiBoundary || /(?:Resource|DTO|Dto|Payload|Response|Request)$/.test(owner)
    const inspect = (node: ts.Node, members: string[]) => {
      if (ts.isPropertySignature(node)) {
        const member =
          ts.isIdentifier(node.name) || ts.isStringLiteral(node.name)
            ? node.name.text
            : node.name.getText(parsed)
        const identity = `${owner}.${[...members, member].join('.')}`
        const documentation = node
          .getFullText(parsed)
          .slice(0, node.getStart(parsed) - node.getFullStart())
        const hasUrlSemantics =
          declaredBoundary || /\b(?:URL|URI|https?:\/\/)\b/i.test(documentation)
        if (
          /^(?:image|images|avatar|avatars)(?:Url|URL)?$/.test(member) &&
          isUrlStringType(node.type) &&
          hasUrlSemantics &&
          !keptLegacyUrlFields.has(`${relativePath}:${identity}`)
        ) {
          warnings.push({
            path: relativePath,
            detail: `field '${identity}' carries a URL string; name new URL fields '*_url'`,
          })
        }
        if (node.type) ts.forEachChild(node.type, (child) => inspect(child, [...members, member]))
        return
      }
      ts.forEachChild(node, (child) => inspect(child, members))
    }
    ts.forEachChild(declaration, (node) => inspect(node, []))
  }
  const visit = (node: ts.Node) => {
    if (ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node)) {
      inspectDeclaration(node)
      return
    }
    ts.forEachChild(node, visit)
  }
  visit(parsed)
  return warnings
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
  const parsed = ts.createSourceFile('module.ts', source, ts.ScriptTarget.Latest, true)
  for (const statement of parsed.statements) {
    if (
      ts.isExportDeclaration(statement) &&
      statement.exportClause &&
      ts.isNamedExports(statement.exportClause)
    ) {
      for (const element of statement.exportClause.elements) exported.add(element.name.text)
    }
  }
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
  if (fileName.endsWith('.shim.ts')) return null
  if (!/^use[A-Z]/.test(fileName) || fileName.endsWith('.spec.ts')) return null
  if (Object.hasOwn(keptComposableFamilies, relativePath)) return null
  // P4 compatibility shims (same-name legacy-path bridges, TTL delete-after-P5
  // owner p4-admin-shared) are pure `export * from` re-exports, not declarations.
  // They carry the moved module's stem and re-export only; symbol-alignment
  // judges declarations, not bridges.
  if (source.includes('compatibility shim') && /export\s+\*\s+from/.test(source)) return null
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

function collectViolations(): {
  violations: Violation[]
  urlWarnings: Array<{ path: string; detail: string }>
} {
  const violations: Violation[] = []
  const urlWarnings: Array<{ path: string; detail: string }> = []
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
          const stutter = checkAntiStutterStem(relative, stem, 'anti-stutter', {
            controlColocatedSfc: isControlColocatedSfc(relative, entry),
          })
          if (stutter) violations.push(stutter)
          const same = checkSameName(relative, dir, entry)
          if (same) violations.push(same)
          const acronym = checkAcronym(relative, entry)
          if (acronym) violations.push(acronym)
          const stale = checkStaleWorkspacePrefix(relative, entry)
          if (stale) violations.push(stale)
          const roleSuffix = checkRoleSuffix(relative, entry)
          if (roleSuffix) violations.push(roleSuffix)
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
          const roleSuffix = checkRoleSuffix(relative, entry)
          if (roleSuffix) violations.push(roleSuffix)
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
        const roleSuffix = checkRoleSuffix(relative, entry)
        if (roleSuffix) violations.push(roleSuffix)
        const barrel = checkIndexBarrel(relative, absolute, entry)
        if (barrel) violations.push(barrel)
        try {
          urlWarnings.push(
            ...collectUrlSuffixWarnings(relative, entry, readFileSync(absolute, 'utf8')),
          )
        } catch {
          // Unreadable files stay out of the advisory URL-suffix scope.
        }
      } else if (/\.(css|scss|svg|png|jpg|jpeg|webp)$/.test(entry)) {
        const style = checkStyleAsset(relative, entry)
        if (style) violations.push(style)
      }
    }
  }
  visit(SRC_ROOT, SRC_ROOT)
  visit(SCRIPTS_ROOT, SCRIPTS_ROOT)
  return {
    violations: violations.sort((a, b) => a.path.localeCompare(b.path)),
    urlWarnings: urlWarnings.sort((a, b) => a.path.localeCompare(b.path)),
  }
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
    // --- gallery controller remainder (issue #692 follow-up) ---------------
    // `galleryController.ts` repeats ancestor `gallery` plus the `Controller`
    // responsibility kind; any other single-word remainder still flags.
    [
      'gallery-controller-remainder-allowed',
      checkAntiStutterStem(
        'features/board/ui/gallery/controllers/galleryController.ts',
        'galleryController',
        'anti-stutter',
      ),
      null,
    ],
    [
      'gallery-other-remainder-still-flagged',
      checkAntiStutterStem(
        'features/board/ui/gallery/controllers/galleryHelper.ts',
        'galleryHelper',
        'anti-stutter',
      )?.kind,
      'anti-stutter',
    ],
    // --- Canvas core graph keep (explicit file list, not dir regex) ------
    // Pinned renames pass; a future graph/graph*.ts stutter still flags.
    [
      'canvas-graph-keep-exempt',
      checkAntiStutterStem(
        'features/canvas/core/graph/graphStore.ts',
        'graphStore',
        'anti-stutter',
      ),
      null,
    ],
    [
      'canvas-graph-future-stutter-still-flagged',
      checkAntiStutterStem(
        'features/canvas/core/graph/graphFoo.ts',
        'graphFoo',
        'anti-stutter',
      )?.kind,
      'anti-stutter',
    ],
    // --- Canvas graph keep staleness -----------------------------------
    // Keep entries with no file on disk fail with a cleanup prompt (probe
    // injected so the negative control never touches the worktree).
    [
      'canvas-keep-stale-clean',
      collectCanvasKeepStale(() => true),
      [],
    ],
    [
      'canvas-keep-stale-detects-missing',
      collectCanvasKeepStale(
        (relativePath) => relativePath !== 'features/canvas/core/graph/graphStore.ts',
      ),
      ['features/canvas/core/graph/graphStore.ts'],
    ],
    // --- ADR-0013 tool zones -----------------------------------------
    // session/ files are not judged for anti-stutter; control-colocated
    // SFCs may share their control directory's stem; everything else under
    // tools/ stays governed.
    [
      'session-zone-exempt',
      checkAntiStutterStem(
        'features/canvas/tools/retouch/session/retouchViewModel.ts',
        'retouchViewModel',
        'anti-stutter',
      ),
      null,
    ],
    [
      'control-colocated-sfc-exempt',
      checkAntiStutterStem(
        'features/canvas/tools/retouch/ui/curves/CurvesControl.vue',
        'CurvesControl',
        'anti-stutter',
        { controlColocatedSfc: true },
      ),
      null,
    ],
    [
      'control-colocated-other-ancestor-still-flagged',
      checkAntiStutterStem(
        'features/canvas/tools/retouch/ui/curves/RetouchCurves.vue',
        'RetouchCurves',
        'anti-stutter',
        { controlColocatedSfc: true },
      )?.kind,
      'anti-stutter',
    ],
    [
      'control-colocated-ts-still-flagged',
      checkAntiStutterStem(
        'features/canvas/tools/retouch/ui/curves/curvesHelpers.ts',
        'curvesHelpers',
        'anti-stutter',
      )?.kind,
      'anti-stutter',
    ],
    [
      'control-colocated-same-name-exempt',
      checkSameName(
        'features/canvas/tools/retouch/ui/chrome/Chrome.vue',
        'features/canvas/tools/retouch/ui/chrome',
        'Chrome.vue',
      ),
      null,
    ],
    [
      'session-zone-same-name-still-flagged',
      checkSameName(
        'features/canvas/tools/retouch/session/chrome/Chrome.vue',
        'features/canvas/tools/retouch/session/chrome',
        'Chrome.vue',
      )?.kind,
      'same-name',
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
      checkGenericName('core/i18n/locales/en/common.ts', 'common.ts', false),
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
    ['style-kebab-ok', checkStyleAsset('core/motion/dialog-motion.css', 'dialog-motion.css'), null],
    [
      'style-camel-bad',
      checkStyleAsset('core/motion/dialogMotion.css', 'dialogMotion.css')?.kind,
      'style-asset',
    ],
    [
      'shared-dir-needs-no-generic-keep',
      checkGenericName('features/account/presentation/shared', 'shared', true),
      null,
    ],
    [
      'generic-flagged',
      checkGenericName('features/x/utils.ts', 'utils.ts', false)?.kind,
      'generic-name',
    ],
    // --- rule 15: role-suffix ----------------------------------------
    // New `*Manager/*Helper/*Util/*Data` files fail; specs mirroring their
    // subject and documented pre-existing keeps stay green.
    [
      'role-suffix-manager',
      checkRoleSuffix('features/x/boardManager.ts', 'boardManager.ts')?.kind,
      'role-suffix',
    ],
    [
      'role-suffix-helper',
      checkRoleSuffix('features/x/canvasHelper.ts', 'canvasHelper.ts')?.kind,
      'role-suffix',
    ],
    [
      'role-suffix-util',
      checkRoleSuffix('features/x/imageUtil.ts', 'imageUtil.ts')?.kind,
      'role-suffix',
    ],
    [
      'role-suffix-data',
      checkRoleSuffix('features/x/sessionData.ts', 'sessionData.ts')?.kind,
      'role-suffix',
    ],
    [
      'role-suffix-vue',
      checkRoleSuffix('features/x/SessionManager.vue', 'SessionManager.vue')?.kind,
      'role-suffix',
    ],
    [
      'role-suffix-ok',
      checkRoleSuffix('features/x/sessionCoordinator.ts', 'sessionCoordinator.ts'),
      null,
    ],
    [
      'role-suffix-metadata-out-of-scope',
      checkRoleSuffix('api/mediaAssetMetadata.ts', 'mediaAssetMetadata.ts'),
      null,
    ],
    [
      'role-suffix-utils-plural-out-of-scope',
      checkRoleSuffix('features/canvas/tools/retouch/core/imageDataUtils.ts', 'imageDataUtils.ts'),
      null,
    ],
    [
      'role-suffix-spec-mirror-exempt',
      checkRoleSuffix(
        'features/admin/models/core/catalogData.unit.spec.ts',
        'catalogData.unit.spec.ts',
      ),
      null,
    ],
    [
      'role-suffix-kept-data',
      checkRoleSuffix('features/admin/models/core/catalogData.ts', 'catalogData.ts'),
      null,
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
      'symbol-alignment-named-reexport',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export {\n useThing\n} from "./implementation"',
      ),
      null,
    ],
    [
      'symbol-alignment-aliased-export',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'const useInternal = () => 1; export { useInternal as useThing }',
      ),
      null,
    ],
    [
      'symbol-alignment-reexport-alias-mismatch',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        'export { useThing as useOther } from "./implementation"',
      )?.kind,
      'symbol-alignment',
    ],
    [
      'symbol-alignment-commented-reexport',
      checkSymbolAlignmentSource(
        'features/example/useThing.ts',
        'useThing.ts',
        '// export { useThing } from "./implementation"',
      )?.kind,
      'symbol-alignment',
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
    // --- rule 16: *_url advisory -------------------------------------
    // New `image/avatar` URL fields warn; `*_url` names, consumers, specs,
    // and grandfathered backend shapes stay silent. Warn-only: the check
    // never fails the lane.
    [
      'url-suffix-new-avatar-warns',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'export interface ExampleResource {\n  avatar: string\n}\n',
      ).length,
      1,
    ],
    [
      'url-suffix-optional-avatar-warns',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'export interface ExampleResource {\n  avatar?: string | undefined\n}\n',
      ).length,
      1,
    ],
    [
      'url-suffix-suffixed-name-ok',
      collectUrlSuffixWarnings(
        'features/example/view.ts',
        'view.ts',
        'export interface ExampleView {\n  avatarUrl: string\n}\n',
      ),
      [],
    ],
    [
      'url-suffix-wire-camelcase-warns',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'export interface ExampleResource { imageUrl: string; avatarUrl?: string[] }',
      ).length,
      2,
    ],
    [
      'url-suffix-relocated-api-boundary-warns',
      collectUrlSuffixWarnings(
        'api/newUser.ts',
        'newUser.ts',
        'export interface User { image: string; avatar: string; avatarUrl?: string }',
      ).length,
      3,
    ],
    [
      'url-suffix-api-boundary-warns',
      collectUrlSuffixWarnings(
        'api/newUser.ts',
        'newUser.ts',
        'export interface User { image: string; avatar: string; imageUrl?: string }',
      ).length,
      3,
    ],
    [
      'url-suffix-wire-snakecase-ok',
      collectUrlSuffixWarnings(
        'api/newUser.ts',
        'newUser.ts',
        'export interface User { image_url: string; avatar_url: string; image_urls?: string[] }',
      ),
      [],
    ],
    [
      'url-suffix-consumer-ok',
      collectUrlSuffixWarnings(
        'features/example/view.ts',
        'view.ts',
        'export const label = user.avatar || ""\n',
      ),
      [],
    ],
    [
      'url-suffix-spec-mirror-exempt',
      collectUrlSuffixWarnings(
        'features/example/resource.unit.spec.ts',
        'resource.unit.spec.ts',
        'export interface ExampleResource {\n  avatar: string\n}\n',
      ),
      [],
    ],
    [
      'url-suffix-kept-shape-exempt',
      collectUrlSuffixWarnings(
        'features/admin/users/session/resource.ts',
        'resource.ts',
        'export interface AdminUserListItem {\n  avatar: string\n}\n',
      ),
      [],
    ],
    [
      'url-suffix-existing-file-new-owner-warns',
      collectUrlSuffixWarnings(
        'features/admin/users/session/resource.ts',
        'resource.ts',
        'export type AdminUserListItem = { avatar: string }; export interface NewUser { image: string; avatar?: string | null }',
      ).length,
      2,
    ],
    [
      'url-suffix-existing-owner-new-member-warns',
      collectUrlSuffixWarnings(
        'features/admin/users/session/resource.ts',
        'resource.ts',
        'export type AdminUserListItem = { avatar: string; image: string }',
      ).length,
      1,
    ],
    [
      'url-suffix-array-and-single-line-warns',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'export interface ExampleResource { images?: string[]; avatars: ReadonlyArray<string> | null }',
      ).length,
      2,
    ],
    [
      'url-suffix-nested-owner-not-exempt',
      collectUrlSuffixWarnings(
        'features/admin/users/session/resource.ts',
        'resource.ts',
        'export type AdminUserListItem = { details: { avatar: string } }',
      ).length,
      1,
    ],
    [
      'url-suffix-display-label-not-url',
      collectUrlSuffixWarnings(
        'features/example/display.ts',
        'display.ts',
        'interface Labels { image: string; avatar: string }',
      ),
      [],
    ],
    [
      'url-suffix-documented-url-outside-boundary',
      collectUrlSuffixWarnings(
        'features/example/display.ts',
        'display.ts',
        'interface Labels { /** URL of the preview. */ image: string }',
      ).length,
      1,
    ],
    [
      'url-suffix-nonstring-member-ok',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'interface ExampleResource { image: Blob; avatars: number[]; images: string | Blob }',
      ),
      [],
    ],
    [
      'url-suffix-method-parameter-ok',
      collectUrlSuffixWarnings(
        'features/example/resource.ts',
        'resource.ts',
        'interface ExampleResource { setAvatar(avatar: string): void }',
      ),
      [],
    ],
    [
      'url-suffix-generated-exclusion',
      collectUrlSuffixWarnings(
        'api/generated-query/model/user.ts',
        'user.ts',
        'interface User { avatar: string }',
      ),
      [],
    ],
    // --- generic keep staleness (existence + still-hit) ---------------
    // Every kept* map is pinned by both halves: a deleted file fails the
    // existence half, a fixed file fails the still-hit half.
    ['keep-generic-still-hits', genericKeepStillHits('core/utils'), true],
    ['keep-generic-unneeded', genericKeepStillHits('features/x/coordinator.ts'), false],
    [
      'keep-role-still-hits',
      roleSuffixKeepStillHits('features/admin/models/core/catalogData.ts'),
      true,
    ],
    ['keep-role-unneeded', roleSuffixKeepStillHits('features/x/coordinator.ts'), false],
    [
      'keep-composable-still-hits',
      composableKeepStillHits('export type A = string', 'useSettingsFeedback.ts'),
      true,
    ],
    [
      'keep-composable-unneeded',
      composableKeepStillHits('export function useThing() {}', 'useThing.ts'),
      false,
    ],
    [
      'keep-multifamily-still-hits',
      multiFamilyKeepStillHits(
        'export type Alpha = string\nexport interface Beta { value: number }',
      ),
      true,
    ],
    [
      'keep-multifamily-unneeded',
      multiFamilyKeepStillHits('export type Single = string'),
      false,
    ],
    [
      'keep-barrel-still-hits',
      barrelKeepStillHits('export const total = (a, b) => a + b\n', 'features/example/index.ts'),
      true,
    ],
    [
      'keep-barrel-unneeded',
      barrelKeepStillHits("export { a } from './a'\n", 'features/example/index.ts'),
      false,
    ],
    [
      'keep-legacy-url-still-hits',
      legacyUrlKeepStillHits('export interface User { avatar?: string }', 'User.avatar'),
      true,
    ],
    [
      'keep-legacy-url-unneeded',
      legacyUrlKeepStillHits('export interface User { name: string }', 'User.avatar'),
      false,
    ],
    [
      'keep-stale-all-missing',
      collectKeepStale(
        () => false,
        () => null,
      ).length,
      Object.keys(keptGenericOwnership).length +
        Object.keys(keptRoleSuffixOwnership).length +
        Object.keys(keptComposableFamilies).length +
        Object.keys(keptMultiFamilyTypes).length +
        Object.keys(keptBarrelBoundaries).length +
        keptLegacyUrlFields.size,
    ],
    [
      'keep-stale-clean-probe',
      collectKeepStale(
        () => true,
        () => 'placeholder',
      ).filter((entry) => entry.includes('(missing)')),
      [],
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
  const { violations, urlWarnings } = collectViolations()
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

  for (const stale of collectCanvasKeepStale()) {
    failed = true
    console.error(
      `[naming-keep:stale] ${stale} no longer exists; remove it from CANVAS_CORE_GRAPH_STUTTER_KEEP`,
    )
  }

  for (const stale of collectKeepStale()) {
    failed = true
    console.error(`[naming-keep:stale] ${stale}; remove it from the named kept* map`)
  }

  for (const warning of urlWarnings) {
    console.warn(`[naming-url-suffix:warn] ${warning.path} :: ${warning.detail}`)
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
