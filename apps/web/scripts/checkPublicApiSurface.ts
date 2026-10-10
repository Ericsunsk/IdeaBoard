import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { format, resolveConfig } from 'prettier'
import { parse } from 'vue/compiler-sfc'
import { listSurfaceEntrypoints, loadSeamInventory } from './seamInventory.ts'

/**
 * Public API surface ratchet for declared feature entrypoints.
 *
 * Entrypoints come from the seam inventory (`docs/frontend-seam-inventory.md`,
 * `surfaceBaseline: true`); `knip.jsonc` `entry` is no longer read here, so
 * activation/retirement flips apply to the ratchet atomically with the
 * inventory. Those barrels use a mix of explicit `export { ... }`
 * and `export *`, and an `export *` chain silently widens the public surface
 * whenever an internal module grows a new export — a leak that is invisible in
 * review and expensive to retract later (consumers start depending on it).
 *
 * This check expands every entrypoint to its full transitive symbol set and
 * compares it against `publicApiSurface.baseline.json`. Adding or removing a
 * public symbol must therefore be a deliberate edit to the baseline, which is
 * what makes the surface reviewable in a diff.
 *
 * Every active canonical entry, including pure subpaths, has a baseline.
 * Existing cascading entry contracts remain covered. Newly covered surfaces
 * warn in P0; `--strict` activates the P1 gate. Missing active entries always fail.
 *
 * ## Barrel presence
 *
 * The same gate also ratchets feature shape: every directory under
 * `src/features` must either ship a root `index.ts` barrel or be recorded in
 * `INTENTIONAL_FLATS` with its reason (`admin` verticals, `canvas`
 * intra-feature seams). A new feature without a barrel fails this check until
 * it gains one or documents its exception.
 *
 * ## Note on scope
 *
 * A barrel that re-exports only an explicit `export { ... }` list does not
 * transitively expose that module's internals — an `export *` on top of it is
 * what leaks. Sealing a split entry file with an explicit export list is
 * therefore how the modules behind it stay private, and this check will not
 * flag additions inside those sealed modules (they never reach the surface).
 *
 * ## Aliased re-exports count
 *
 * `resolveModule` used to bail out on any specifier that was not relative, so
 * `export * from '@/features/x/internal'` resolved to nothing and its symbols
 * never entered the baseline — the exact leak the ratchet is for. `@/` and
 * `src/` now resolve against `src/` as `tsconfig.app.json`
 * declares it. An `export *` that still cannot be resolved is reported rather
 * than skipped, because a silently dropped re-export is an invisible surface.
 *
 * Run `npm run api:surface:update` to regenerate the baseline after an
 * intentional change.
 */

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const baselinePath = path.join(webRoot, 'publicApiSurface.baseline.json')

/**
 * Resolve an `export ... from` specifier.
 *
 * `@/` (and `src/`) were previously rejected outright, so a barrel that
 * re-exported through the alias contributed **nothing** to the baseline — which
 * is precisely the `export *` leak this ratchet exists to catch: widening
 * `export * from '@/features/x/internal'` silently added public symbols and the
 * check reported no drift. `tsconfig.app.json` maps `@/*` to
 * `./src/*`, so the alias is resolved here too.
 */
function resolveModule(root: string, fromFile: string, specifier: string): string | null {
  const candidates = specifier.startsWith('@/')
    ? [path.join(root, 'src', specifier.slice(2))]
    : specifier.startsWith('src/')
      ? [path.join(root, specifier)]
      : specifier.startsWith('.')
        ? [path.resolve(path.dirname(fromFile), specifier)]
        : []
  if (candidates.length === 0) return null
  const base = candidates[0] as string
  const resolved = [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    `${base}.vue`,
    path.join(base, 'index.ts'),
  ].find((file) => fs.existsSync(file) && fs.statSync(file).isFile())
  return resolved ?? null
}

/** Expand an entrypoint to every symbol it exposes, following `export *`. */
function scanEntrypoint(
  root: string,
  file: string,
  seen = new Set<string>(),
  unresolved: string[] = [],
  cache = new Map<string, Set<string>>(),
): Set<string> {
  const symbols = cache.get(file) ?? new Set<string>()
  if (seen.has(file)) return symbols
  cache.set(file, symbols)
  seen.add(file)

  let content = fs.readFileSync(file, 'utf8')
  if (file.endsWith('.vue')) {
    symbols.add('default')
    const { descriptor, errors } = parse(content, { filename: file })
    if (errors.length) unresolved.push(`${path.relative(root, file)}: invalid Vue SFC`)
    content = [descriptor.script?.content, descriptor.scriptSetup?.content]
      .filter(Boolean)
      .join('\n')
  }
  const source = ts.createSourceFile(file, content, ts.ScriptTarget.Latest, true)
  // Seed direct/named declarations before following branches so cycles can
  // resolve already declared symbols without dropping ancestor tracking.
  for (const statement of source.statements) collectStatementSymbols(statement, symbols)
  for (const statement of source.statements) {
    if (ts.isExportDeclaration(statement)) {
      if (
        statement.exportClause &&
        statement.moduleSpecifier &&
        ts.isStringLiteral(statement.moduleSpecifier)
      ) {
        const specifier = statement.moduleSpecifier.text
        const local =
          specifier.startsWith('.') || specifier.startsWith('@/') || specifier.startsWith('src/')
        const target = local
          ? resolveModule(root, file, specifier)
          : ts.resolveModuleName(
              specifier,
              file,
              { moduleResolution: ts.ModuleResolutionKind.Bundler },
              ts.sys,
            ).resolvedModule?.resolvedFileName
        if (!target)
          unresolved.push(`${path.relative(root, file)} -> ${statement.moduleSpecifier.text}`)
        else {
          const nested = local
            ? scanEntrypoint(root, target, new Set(seen), unresolved, cache)
            : null
          if (nested && ts.isNamedExports(statement.exportClause)) {
            for (const element of statement.exportClause.elements) {
              const imported = element.propertyName?.text ?? element.name.text
              if (!nested.has(imported))
                unresolved.push(
                  `${path.relative(root, file)} -> ${statement.moduleSpecifier.text} has no export ${imported}`,
                )
            }
          }
        }
      }
      if (
        !statement.exportClause &&
        statement.moduleSpecifier &&
        ts.isStringLiteral(statement.moduleSpecifier)
      ) {
        const target = resolveModule(root, file, statement.moduleSpecifier.text)
        if (!target)
          unresolved.push(`${path.relative(root, file)} -> ${statement.moduleSpecifier.text}`)
        else {
          const nested = scanEntrypoint(root, target, seen, unresolved, cache)
          nested.forEach((name) => {
            if (name !== 'default') symbols.add(name)
          })
        }
      }
    }
  }
  return symbols
}

function collectStatementSymbols(statement: ts.Statement, symbols: Set<string>): void {
  if (ts.isExportAssignment(statement)) symbols.add('default')
  else if (ts.isExportDeclaration(statement) && statement.exportClause) {
    if (ts.isNamedExports(statement.exportClause))
      statement.exportClause.elements.forEach((element) => symbols.add(element.name.text))
    else if (ts.isNamespaceExport(statement.exportClause))
      symbols.add(statement.exportClause.name.text)
  } else if (
    ts.canHaveModifiers(statement) &&
    ts.getModifiers(statement)?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword)
  ) {
    if (
      ts.getModifiers(statement)?.some((modifier) => modifier.kind === ts.SyntaxKind.DefaultKeyword)
    )
      symbols.add('default')
    else if (ts.isVariableStatement(statement))
      statement.declarationList.declarations.forEach((declaration) =>
        collectBindingNames(declaration.name, symbols),
      )
    else if ('name' in statement && statement.name && ts.isIdentifier(statement.name as ts.Node))
      symbols.add((statement.name as ts.Identifier).text)
  }
}

function collectBindingNames(name: ts.BindingName, symbols: Set<string>): void {
  if (ts.isIdentifier(name)) symbols.add(name.text)
  else
    name.elements.forEach((element) => {
      if (ts.isBindingElement(element)) collectBindingNames(element.name, symbols)
    })
}

function collectSurfaceFor(
  root: string,
  entrypoints: string[],
): { surface: Record<string, string[]>; unresolved: string[] } {
  const surface: Record<string, string[]> = {}
  const unresolved: string[] = []
  for (const entrypoint of entrypoints) {
    const absolute = path.resolve(root, entrypoint)
    if (!fs.existsSync(absolute)) {
      unresolved.push(`${entrypoint}: missing active entry`)
      continue
    }
    const symbols = scanEntrypoint(root, absolute, new Set<string>(), unresolved)
    surface[entrypoint] = [...symbols].sort()
  }
  return { surface, unresolved }
}

const FEATURES_ROOT = path.join(webRoot, 'src', 'features')

/** Feature roots expected to expose an `index.ts` public entrypoint. */
const EXPECTED_FEATURE_BARRELS = [
  'account',
  'auth',
  'board',
  'legal',
  'library',
  'search',
  'workspace',
]

/**
 * Feature roots that intentionally ship without a root barrel, with the
 * reason recorded. `admin` is a set of vertical contexts with no single
 * public seam (each area owns its resource boundary); `canvas` crosses its
 * intra-feature seams (the declared canvas entry barrels in the inventory)
 * instead of a root barrel. Light flats such as `legal` need no exception:
 * they ship a root barrel despite their flat interior.
 */
const INTENTIONAL_FLATS: Record<string, string> = {
  admin: 'vertical contexts own per-area resource boundaries; no single feature barrel',
  canvas: 'intra-feature seams are the declared inventory entry barrels; no root barrel by design',
}

/** Flag feature roots missing a barrel without a documented exception. */
function collectFeatureBarrelProblems(): string[] {
  const found = new Set(
    fs
      .readdirSync(FEATURES_ROOT, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name),
  )
  const barrelProblems: string[] = []
  for (const name of [...found].sort()) {
    if (fs.existsSync(path.join(FEATURES_ROOT, name, 'index.ts'))) continue
    if (!(name in INTENTIONAL_FLATS)) {
      barrelProblems.push(
        `Feature "${name}" has no index.ts barrel and no INTENTIONAL_FLATS exception — add a barrel or document the exception.`,
      )
    }
  }
  for (const name of EXPECTED_FEATURE_BARRELS) {
    if (!found.has(name)) {
      barrelProblems.push(`Expected feature "${name}" is missing from src/features.`)
    } else if (!fs.existsSync(path.join(FEATURES_ROOT, name, 'index.ts'))) {
      barrelProblems.push(`Feature "${name}" lost its index.ts barrel.`)
    }
  }
  return barrelProblems
}

/** Pure drift comparison shared by the production run and `--self-test` fixtures. */
function diffSurface(
  current: Record<string, string[]>,
  baseline: Record<string, string[]>,
): string[] {
  const problems: string[] = []
  for (const entrypoint of Object.keys(current)) {
    if (!(entrypoint in baseline)) {
      problems.push(`Entrypoint "${entrypoint}" is declared but absent from the baseline.`)
    }
  }
  for (const entrypoint of Object.keys(baseline)) {
    if (!(entrypoint in current)) {
      problems.push(`Entrypoint "${entrypoint}" is in the baseline but no longer declared.`)
    }
  }
  for (const [entrypoint, names] of Object.entries(current)) {
    const expected = baseline[entrypoint]
    if (!expected) continue
    const currentSet = new Set(names)
    const baselineSet = new Set(expected)
    const added = names.filter((name) => !baselineSet.has(name))
    const removed = expected.filter((name) => !currentSet.has(name))
    if (added.length > 0) {
      problems.push(
        `"${entrypoint}" exposes ${added.length} new public symbol(s): ${added.join(', ')}`,
      )
    }
    if (removed.length > 0) {
      problems.push(
        `"${entrypoint}" no longer exposes ${removed.length} symbol(s): ${removed.join(', ')}`,
      )
    }
  }
  return problems
}

function unresolvedProblems(unresolved: string[]): string[] {
  return unresolved.map(
    (entry) =>
      `Unresolved \`export *\` (${entry}). Its symbols are missing from the surface; fix the specifier or seal the barrel with an explicit export list.`,
  )
}

/**
 * Positive/negative fixtures for the surface ratchet (P0 deliverable B).
 *
 * Runs in a disposable temp workspace so the real baseline is untouched:
 * - positive: unchanged entry matches its baseline (valid public entry);
 * - negative: `export *` leak widening the surface is flagged (invalid leak);
 * - negative: `export *` to a missing module is reported, not skipped;
 * - positive: sealed barrel (explicit export list) stays green while the
 *   module behind it grows (pure-subpath-style sealing).
 */
function runSurfaceSelfTest(): void {
  let failures = 0
  const check = (name: string, condition: boolean, detail = '') => {
    if (condition) {
      console.log(`✓ Surface fixture: ${name}.`)
    } else {
      console.error(`✗ Surface fixture failed: ${name}. ${detail}`)
      failures += 1
    }
  }

  const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'ideaboard-surface-')))
  const writeWorkspaceFile = (relative: string, content: string) => {
    const absolute = path.join(workspace, relative)
    fs.mkdirSync(path.dirname(absolute), { recursive: true })
    fs.writeFileSync(absolute, content)
  }
  try {
    const entry = 'src/seam/index.ts'
    // Valid public entry: explicit symbol + `export *` chain.
    writeWorkspaceFile(entry, `export const keep = 1;\nexport * from './internal';\n`)
    writeWorkspaceFile('src/seam/internal.ts', `export const leaked = 1;\n`)
    // Sealed barrel: explicit list, so growth behind it never leaks.
    writeWorkspaceFile('src/sealed/index.ts', `export { sealed } from './impl';\n`)
    writeWorkspaceFile('src/sealed/impl.ts', `export const sealed = 1;\n`)

    const first = collectSurfaceFor(workspace, [entry, 'src/sealed/index.ts'])
    check(
      'valid public entry expands its transitive symbols',
      first.surface[entry]?.join(',') === 'keep,leaked',
      JSON.stringify(first.surface),
    )
    check(
      'no unresolved star exports in fixtures',
      first.unresolved.length === 0,
      first.unresolved.join('; '),
    )
    check(
      'unchanged entry matches its baseline',
      diffSurface(first.surface, first.surface).length === 0,
    )

    // Negative: widening the `export *` chain must read as drift.
    writeWorkspaceFile(
      'src/seam/internal.ts',
      `export const leaked = 1;\nexport const sneaked = 2;\n`,
    )
    const widened = collectSurfaceFor(workspace, [entry])
    const drift = diffSurface(widened.surface, { [entry]: ['keep', 'leaked'] })
    check(
      'export * widening is flagged as drift',
      drift.length === 1 && (drift[0]?.includes('sneaked') ?? false),
      drift.join('; '),
    )

    // Negative: dangling `export *` is reported, never silently skipped.
    writeWorkspaceFile(entry, `export const keep = 1;\nexport * from './missing';\n`)
    const dangling = collectSurfaceFor(workspace, [entry])
    check(
      'unresolved export * is reported',
      dangling.unresolved.length === 1,
      JSON.stringify(dangling.surface),
    )

    // Positive: sealed barrel stays green while the module behind it grows.
    writeWorkspaceFile(
      'src/sealed/impl.ts',
      `export const sealed = 1;\nexport const privateGrowth = 2;\n`,
    )
    const sealed = collectSurfaceFor(workspace, ['src/sealed/index.ts'])
    check(
      'sealed barrel hides growth behind an explicit export list',
      diffSurface(sealed.surface, { 'src/sealed/index.ts': ['sealed'] }).length === 0,
      JSON.stringify(sealed.surface),
    )
    const missing = collectSurfaceFor(workspace, ['src/missing/pure.ts'])
    check('missing active pure subpath is an error', missing.unresolved.length === 1)
    writeWorkspaceFile('src/named.ts', `export { missing } from './absent';\n`)
    check(
      'named re-export resolves its target',
      collectSurfaceFor(workspace, ['src/named.ts']).unresolved.length === 1,
    )
    writeWorkspaceFile('src/named.ts', `export { missing } from './sealed/impl';\n`)
    check(
      'named re-export checks the exported symbol',
      collectSurfaceFor(workspace, ['src/named.ts']).unresolved.some((problem) =>
        problem.includes('no export missing'),
      ),
    )
    writeWorkspaceFile('src/cycle-a.ts', `export { b } from './cycle-b';\nexport const a = 1;\n`)
    writeWorkspaceFile('src/cycle-b.ts', `export { a } from './cycle-a';\nexport const b = 1;\n`)
    const cycle = collectSurfaceFor(workspace, ['src/cycle-a.ts'])
    check(
      'named export cycle terminates with valid symbols',
      cycle.unresolved.length === 0 && cycle.surface['src/cycle-a.ts']?.join(',') === 'a,b',
    )
    writeWorkspaceFile(
      'src/pure.ts',
      `/* export const fake = 1 */\nexport\nconst actual = 1, second = 2;\nexport type Shape = string;\n`,
    )
    const pure = collectSurfaceFor(workspace, ['src/pure.ts'])
    writeWorkspaceFile(
      'src/Panel.vue',
      `<script lang="ts">export type PanelVariant = 'plain'; export const panelDefault = 'plain'; export default { name: 'Panel' }</script><template><div /></template>`,
    )
    check(
      'Vue public named exports join the component default',
      collectSurfaceFor(workspace, ['src/Panel.vue']).surface['src/Panel.vue']?.join(',') ===
        'PanelVariant,default,panelDefault',
    )
    check(
      'AST handles multiline/multiple declarations and ignores comments',
      pure.surface['src/pure.ts']?.join(',') === 'Shape,actual,second',
    )
  } finally {
    fs.rmSync(workspace, { recursive: true, force: true })
  }
  if (failures > 0) process.exit(1)
  console.log('\n✓ All public API surface fixtures passed!')
}

if (process.argv.includes('--self-test')) {
  runSurfaceSelfTest()
  process.exit(0)
}

const seams = loadSeamInventory().seams
const { surface: current, unresolved: unresolvedStarExports } = collectSurfaceFor(
  webRoot,
  listSurfaceEntrypoints(seams),
)
const planned = seams.filter((seam) => seam.status === 'planned')
const sourceContracts = collectSurfaceFor(
  webRoot,
  [...new Set(planned.flatMap((seam) => seam.sourcePaths))].sort(),
)
unresolvedStarExports.push(...sourceContracts.unresolved)
for (const seam of planned) {
  for (const mapping of seam.exportMappings ?? []) {
    if (!sourceContracts.surface[mapping.source]?.includes(mapping.name))
      unresolvedStarExports.push(
        `${seam.path}: source ${mapping.source} has no export ${mapping.name}`,
      )
  }
}

if (process.argv.includes('--update')) {
  if (unresolvedStarExports.length)
    throw new Error(unresolvedProblems(unresolvedStarExports).join('\n'))
  fs.writeFileSync(
    baselinePath,
    await format(JSON.stringify(current), {
      ...(await resolveConfig(baselinePath)),
      parser: 'json',
    }),
  )
  const total = Object.values(current).reduce((sum, names) => sum + names.length, 0)
  console.log(
    `✓ Wrote public API baseline: ${Object.keys(current).length} entrypoint(s), ${total} symbol(s).`,
  )
  process.exit(0)
}

if (!fs.existsSync(baselinePath)) {
  console.error(`✗ Missing public API baseline at ${path.relative(webRoot, baselinePath)}.`)
  console.error('  Run: npm run api:surface:update')
  process.exit(1)
}

const baseline = JSON.parse(fs.readFileSync(baselinePath, 'utf8')) as Record<string, string[]>
const seamsByPath = new Map(seams.map((seam) => [seam.path, seam]))
const differences = diffSurface(current, baseline)
for (const [entry, names] of Object.entries(current)) {
  const declared = seamsByPath.get(entry)?.publicExports
  if (declared && JSON.stringify(names) !== JSON.stringify(declared))
    differences.push(
      `Entrypoint "${entry}" differs from inventory publicExports; review the frozen contract.`,
    )
}
const warnings = differences.filter(
  (problem) =>
    !process.argv.includes('--strict') &&
    [...seamsByPath.values()].some(
      (seam) => seam.surfaceSeverity === 'warn' && problem.includes(`"${seam.path}"`),
    ),
)
const problems = differences.filter((problem) => !warnings.includes(problem))
for (const warning of warnings) console.warn(`  P0 surface advisory: ${warning}`)

problems.push(...collectFeatureBarrelProblems())
problems.push(...unresolvedProblems(unresolvedStarExports))

if (problems.length > 0) {
  console.error('✗ Public API surface drift detected:')
  for (const problem of problems) console.error(`  - ${problem}`)
  console.error('\nIf this change is intentional, run: npm run api:surface:update')
  process.exit(1)
}

const total = Object.values(current).reduce((sum, names) => sum + names.length, 0)
console.log(
  `✓ Public API surface matches the baseline (${Object.keys(current).length} entrypoint(s), ${total} symbol(s)).`,
)
