import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'
import { format, resolveConfig } from 'prettier'
import {
  edgeKey,
  graphEdges,
  findImpureRuntimePaths,
  isOwned,
  loadSeamInventory,
  measureSeam,
  type CruiseModule,
  type SeamRecord,
} from './seamInventory.ts'

const webRoot = fileURLToPath(new URL('..', import.meta.url))

/** Default fan-in/out report path (overridable with `--report=<path>`). */
const DEFAULT_REPORT_PATH = path.join(webRoot, 'seam-fan-report.json')

function reportPathFromArgs(): string {
  const flag = process.argv.find((arg) => arg.startsWith('--report='))
  return flag ? path.resolve(webRoot, flag.slice('--report='.length)) : DEFAULT_REPORT_PATH
}

/** Current checkout SHA recorded in the report. */
function checkpointSha(): string {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: webRoot, encoding: 'utf8' })
  return result.status === 0 ? result.stdout.trim() : 'unknown'
}

/** Workspace-relative `*.shim.ts` files (the only sanctioned alias kind). */
function listShims(root: string): string[] {
  const found: string[] = []
  const visit = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      if (entry.name === 'node_modules' || entry.name === 'generated-query') continue
      const absolute = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        visit(absolute)
        continue
      }
      if (entry.name.endsWith('.shim.ts')) found.push(toPosix(path.relative(root, absolute)))
    }
  }
  visit(path.join(root, 'src'))
  return found.sort()
}

/** The control client module every generated-query fixture points at. */
const CONTROL_PLATFORM_MODULE = 'src/api/generated-query/control/control-platform/control-platform'

/**
 * One module a fixture imports.
 *
 * `stubModule` is workspace-relative and is materialized as an empty module so
 * the import resolves into a real dependency edge — dependency-cruiser needs
 * the dependency to exist before it can judge the rule against it.
 */
type FixtureFile = {
  /** Workspace-relative, POSIX-separated path of the fixture module. */
  filePath: string
  /**
   * Fixture source. `{target:<stubModule>}` expands to a relative specifier
   * that resolves to a generated stub module.
   */
  content: string
}

type BoundaryFixtureCase = {
  name: string
  expectedRule: string
  /**
   * Files to materialize in the fixture workspace. The first entry is the
   * module dependency-cruiser is pointed at; the rest are its imports.
   */
  files: FixtureFile[]
  /**
   * Exercise this check's own Board deep-import scanner (which resolves `@/`
   * aliases itself) instead of a dependency-cruiser rule.
   */
  scannerOnly?: boolean
}

function toPosix(value: string): string {
  return value.split(path.sep).join('/')
}

/**
 * Resolve a module specifier the way the Board scanner does, honouring the
 * `@/` and `src/` aliases that `tsconfig.app.json` declares.
 */
function resolveSourceModule(root: string, sourcePath: string, specifier: string): string | null {
  if (!specifier.startsWith('.') && !specifier.startsWith('@/') && !specifier.startsWith('src/'))
    return null
  const candidate = specifier.startsWith('@/')
    ? path.join(root, 'src', specifier.slice(2))
    : specifier.startsWith('src/')
      ? path.join(root, specifier)
      : path.resolve(path.dirname(sourcePath), specifier)
  const candidates = [
    candidate,
    `${candidate}.ts`,
    `${candidate}.tsx`,
    `${candidate}.vue`,
    path.join(candidate, 'index.ts'),
  ]
  const resolved = candidates.find((file) => fs.existsSync(file) && fs.statSync(file).isFile())
  return resolved ? path.relative(root, resolved).split(path.sep).join('/') : null
}

/**
 * The Board deep-import scanner.
 *
 * `root` is the directory that `src/` hangs off and the cwd dependency-cruiser
 * runs in, so the same function scans the real checkout (production pass) and
 * the disposable fixture workspace (self-test pass).
 */
function scanBoardDeepImports(
  root: string,
  options: { includeFixtures?: boolean } = {},
): Set<string> {
  const edges = new Set<string>()
  const sourceFiles = runDepcruise(root, [
    '--config',
    '.dependency-cruiser.cjs',
    '--output-type',
    'json',
    'src',
  ])
  const modules = JSON.parse(sourceFiles.stdout || '{}') as { modules?: Array<{ source: string }> }
  for (const module of modules.modules ?? []) {
    if (
      !/\.(?:ts|tsx|vue)$/.test(module.source) ||
      module.source.startsWith('src/features/board/') ||
      (options.includeFixtures !== true && module.source.includes('.fixture.'))
    )
      continue
    const absoluteSource = path.join(root, module.source)
    const content = fs.readFileSync(absoluteSource, 'utf8')
    const importPattern = /(?:from\s+|import\s*\(\s*)['"]([^'"]+)['"]/g
    for (const match of content.matchAll(importPattern)) {
      const specifier = match[1]
      if (!specifier) continue
      const target = resolveSourceModule(root, absoluteSource, specifier)
      if (
        !target ||
        !/^src\/features\/board\/(?:core|session|ui|gallery|testing)(?:\/|$)/.test(target)
      )
        continue
      edges.add(`${module.source}|${target}`)
    }
  }
  return edges
}

/**
 * `route-pages-no-direct-generated-query` used to be anchored on
 * `features/<feature>/(components|presentation|pages|app)`, which only matched
 * 13 of the 31 route pages because `[^/]+` cannot cross a `/`. Each fixture
 * below therefore sits in a *different* real layout shape; one directory would
 * have kept the rule green while it was blind.
 */
const routePageFixtureDirs = [
  'src/features/admin/app',
  'src/features/admin/users/ui/table',
  'src/features/admin/settings/ui/pages',
  'src/features/admin/credits/ui/orders',
  'src/features/workspace/layout',
] as const

const horizontalRoots = ['shared', 'core', 'api', 'theme', 'lib', 'components']
const approvedFeatureShims = [
  'components/UserAccountMenu',
  'components/UserCreditBalanceButton',
  'components/userAccountMenu.types',
  'components/SupportChannelQr',
  'constants/contacts',
  'constants/routePaths',
  'constants/legalLinks',
  'domain/imageActions',
  'domain/serviceIcons',
  'domain/serviceRegistry',
  'media/preloadImage',
]
const fixtureCases: BoundaryFixtureCase[] = [
  ...horizontalRoots.flatMap((root) =>
    ['runtime', 'type-only', 're-export'].map<BoundaryFixtureCase>((kind) => ({
      name: `prohibits ${root} ${kind} dependency on a feature`,
      expectedRule: root === 'shared' ? 'shared-not-to-features' : 'horizontal-not-to-features',
      files: [
        {
          filePath: `src/${root}/boundary-feature-${kind}.fixture.ts`,
          content:
            kind === 'runtime'
              ? `import { value } from '{target:src/features/legal/index}'; console.log(value);`
              : kind === 'type-only'
                ? `import type { value } from '{target:src/features/legal/index}'; export type Probe = typeof value;`
                : `export { value } from '{target:src/features/legal/index}';`,
        },
      ],
    })),
  ),
  ...['shared', 'core'].map<BoundaryFixtureCase>((root) => ({
    name: `prohibits an unapproved ${root} shim bypass`,
    expectedRule: root === 'shared' ? 'shared-not-to-features' : 'horizontal-not-to-features',
    files: [
      {
        filePath: `src/${root}/unapproved.shim.ts`,
        content: `export { value } from '{target:src/features/legal/index}';`,
      },
    ],
  })),
  {
    name: 'prohibits admin features from importing the public generated client',
    expectedRule: 'features-admin-no-public-client',
    files: [
      {
        filePath: 'src/features/admin/boundary-admin-public.fixture.ts',
        content: `import { getUser } from '{target:src/api/generated-query/public/user/user}'; console.log(getUser);`,
      },
    ],
  },
  {
    name: 'prohibits non-admin features from importing the control generated client',
    expectedRule: 'non-admin-no-control-client',
    files: [
      {
        filePath: 'src/features/board/boundary-non-admin-control.fixture.ts',
        content: `import { listControlUsers } from '{target:src/api/generated-query/control/control-users/control-users}'; console.log(listControlUsers);`,
      },
    ],
  },
  {
    name: 'detects circular dependencies',
    expectedRule: 'no-circular',
    files: [
      {
        filePath: 'src/shared/circular-a.fixture.ts',
        content: `import { circularB } from './circular-b.fixture'; export const circularA = () => circularB();`,
      },
      {
        filePath: 'src/shared/circular-b.fixture.ts',
        content: `import { circularA } from './circular-a.fixture'; export const circularB = () => circularA();`,
      },
    ],
  },
  {
    name: 'prohibits shared modules from importing concrete feature modules',
    expectedRule: 'shared-not-to-features',
    files: [
      {
        filePath: 'src/shared/boundary-shared-to-feature.fixture.ts',
        content: `import { adminRoutes } from '{target:src/features/admin/app/routes}'; console.log(adminRoutes);`,
      },
    ],
  },
  // Multi-point: one fixture per real admin/workspace layout shape. These
  // fixtures are named `*.fixture.Page.vue` / `*.fixture.Layout.vue` on purpose —
  // the rule now matches route surfaces by that filename, so a `.ts` fixture
  // would no longer prove anything.
  ...routePageFixtureDirs.map<BoundaryFixtureCase>((dir, index) => ({
    name: `prohibits route pages in ${dir} from importing generated-query directly`,
    expectedRule: 'route-pages-no-direct-generated-query',
    files: [
      {
        filePath: `${dir}/boundary-page-to-query-${index}.fixture.Page.vue`,
        content: [
          '<script setup lang="ts">',
          `import { getGetControlMonitorUrl } from '{target:${CONTROL_PLATFORM_MODULE}}'`,
          'console.log(getGetControlMonitorUrl)',
          '</script>',
        ].join('\n'),
      },
    ],
  })),
  {
    name: 'prohibits cross-feature deep imports into internal directories',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/admin/boundary-cross-feature-deep.fixture.ts',
        content: `import { createEmptyBoardSnapshot } from '{target:src/features/board/core/index}'; console.log(createEmptyBoardSnapshot);`,
      },
    ],
  },
  {
    name: 'prohibits aliased imports into Board internals',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/admin/boundary-board-alias.fixture.ts',
        content: `import { createEmptyBoardSnapshot } from '@/features/board/core'; console.log(createEmptyBoardSnapshot);`,
      },
    ],
    scannerOnly: true,
  },
  {
    name: 'prohibits cross-feature deep imports into auth session internals',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/admin/boundary-auth-deep.fixture.ts',
        content: `import { useUserStore } from '{target:src/features/auth/session/userSession}'; console.log(useUserStore);`,
      },
    ],
  },
  {
    name: 'prohibits cross-feature deep imports into workspace selection',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/admin/boundary-workspace-deep.fixture.ts',
        content: `import { getWorkspaceSelectionStore } from '{target:src/features/workspace/selection/workspaceContext}'; console.log(getWorkspaceSelectionStore);`,
      },
    ],
  },
  {
    name: 'prohibits cross-feature deep imports into library ui',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/search/boundary-library-deep.fixture.ts',
        content: `import LibraryFloatingIsland from '{target:src/features/library/ui/LibraryFloatingIsland.vue}'; console.log(LibraryFloatingIsland);`,
      },
    ],
  },
  {
    name: 'prohibits cross-feature deep imports into search session',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/admin/boundary-search-deep.fixture.ts',
        content: `import { screenRuntime } from '{target:src/features/search/session/screenRuntime}'; console.log(screenRuntime);`,
      },
    ],
  },
  {
    name: 'prohibits cross-feature deep imports into notification session',
    expectedRule: 'no-cross-feature-deep-imports',
    files: [
      {
        filePath: 'src/features/workspace/boundary-notification-deep.fixture.ts',
        content: `import { syncInviteAcceptanceQueries } from '{target:src/features/notification/session/useNotifications}'; console.log(syncInviteAcceptanceQueries);`,
      },
    ],
  },
  {
    name: 'prohibits tool core modules from importing session zone modules',
    expectedRule: 'canvas-tool-core-no-zone-imports',
    files: [
      {
        filePath: 'src/features/canvas/tools/retouch/core/boundary-core-session.fixture.ts',
        content: `import { useRetouchSession } from '{target:src/features/canvas/tools/retouch/session/sessionTarget}'; console.log(useRetouchSession);`,
      },
    ],
  },
  {
    name: 'prohibits tool core modules from importing ui zone modules',
    expectedRule: 'canvas-tool-core-no-zone-imports',
    files: [
      {
        filePath: 'src/features/canvas/tools/inpaint/core/boundary-core-ui.fixture.ts',
        content: `import { chromeTarget } from '{target:src/features/canvas/tools/inpaint/ui/chromeTarget.vue}'; console.log(chromeTarget);`,
      },
    ],
  },
  ...['index', 'i18nRuntime'].map<BoundaryFixtureCase>((module) => ({
    name: `prohibits tool core modules from importing the i18n ${module} module`,
    expectedRule: 'canvas-tool-core-no-i18n-runtime',
    files: [
      {
        filePath: `src/features/canvas/tools/retouch/core/boundary-i18n-${module}.fixture.ts`,
        content: `import { i18n } from '{target:src/core/i18n/${module}}'; console.log(i18n);`,
      },
    ],
  })),
  {
    name: 'prohibits tool core modules from importing the retired i18n shim',
    expectedRule: 'canvas-tool-core-no-i18n-runtime',
    files: [
      {
        filePath: 'src/features/canvas/tools/retouch/core/boundary-i18n-shim.fixture.ts',
        content: `import { i18n } from '{target:src/shared/i18n/index.shim.ts}'; console.log(i18n);`,
      },
    ],
  },
  {
    name: 'prohibits cross-tool deep imports bypassing the tool index seam',
    expectedRule: 'no-cross-tool-deep-imports',
    files: [
      {
        filePath: 'src/features/canvas/tools/inpaint/session/boundary-cross-tool-deep.fixture.ts',
        content: `import { useRetouchSession } from '{target:src/features/canvas/tools/retouch/session/sessionTarget}'; console.log(useRetouchSession);`,
      },
    ],
  },
  {
    name: 'prohibits canvas-layer deep imports into inpaint internals',
    expectedRule: 'no-cross-tool-deep-imports',
    files: [
      {
        filePath: 'src/features/canvas/application/boundary-tool-deep.fixture.ts',
        content: `import { useInpaintChromeState } from '{target:src/features/canvas/tools/inpaint/session/inpaintSessionTarget}'; console.log(useInpaintChromeState);`,
      },
    ],
  },
  {
    name: 'prohibits core modules from importing vue',
    expectedRule: 'core-pure-domain-no-ui',
    files: [
      {
        filePath: 'src/features/board/core/boundary-core-vue.fixture.ts',
        content: `import { ref } from 'vue'; console.log(ref);`,
      },
    ],
  },
  {
    name: 'prohibits core modules from importing ui zone modules',
    expectedRule: 'core-pure-domain-no-ui',
    files: [
      {
        filePath: 'src/features/board/core/boundary-core-ui.fixture.ts',
        content: `import { boardUiTarget } from '{target:src/features/board/ui/boardUiTarget.vue}'; console.log(boardUiTarget);`,
      },
    ],
  },
  {
    name: 'prohibits core modules from importing adapter modules',
    expectedRule: 'core-pure-domain-no-ui',
    files: [
      {
        filePath: 'src/features/board/core/boundary-core-adapter.fixture.ts',
        content: `import { boardAdapterTarget } from '{target:src/features/board/adapters/boardAdapterTarget}'; console.log(boardAdapterTarget);`,
      },
    ],
  },
]

function runDepcruise(cwd: string, args: string[]) {
  const result = spawnSync('npx', ['depcruise', ...args], {
    cwd,
    encoding: 'utf8',
    maxBuffer: 50 * 1024 * 1024,
  })
  return result
}

/**
 * Build the disposable workspace the fixture phase runs in.
 *
 * Fixtures used to be written straight into the real `src/` tree and removed in
 * a `finally` block. `finally` does not run on SIGINT/SIGTERM, so Ctrl-C mid-run
 * left `.boundary-*.fixture.*` files inside `src/`. They are dotfiles, not in
 * `.gitignore`, so they showed up in `git status` *and* dependency-cruiser read
 * them on the next run — turning this check into a source of phantom violations
 * that only reproduced after an interrupted run.
 *
 * The fixture phase now runs under `mkdtemp` with `node_modules` symlinked in,
 * mirroring `checkApiGeneratedDrift.ts`. Nothing is written inside the checkout
 * at all, so an interrupted run cannot leave residue in the working tree.
 */
function createFixtureWorkspace(): string {
  // realpath matters: on macOS `os.tmpdir()` is `/var/folders/...` while
  // `process.cwd()` inside the child resolves to `/private/var/folders/...`.
  // dependency-cruiser relativizes rule paths against its baseDir, so a
  // non-realpathed temp path silently stops every rule from matching.
  const tempRoot = fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(), 'ideaboard-boundaries-')))
  const workspace = path.join(tempRoot, 'web')
  fs.mkdirSync(workspace, { recursive: true })

  // dependency-cruiser resolves `from`/`to` paths relative to cwd, so the temp
  // workspace needs the real rule set and the tsconfig the config points at.
  fs.copyFileSync(
    path.join(webRoot, '.dependency-cruiser.cjs'),
    path.join(workspace, '.dependency-cruiser.cjs'),
  )
  fs.copyFileSync(
    path.join(webRoot, 'tsconfig.app.json'),
    path.join(workspace, 'tsconfig.app.json'),
  )
  // `tsconfig.app.json` extends `@vue/tsconfig/tsconfig.dom.json` and resolves
  // `@/*`, both of which live in node_modules.
  fs.symlinkSync(path.join(webRoot, 'node_modules'), path.join(workspace, 'node_modules'), 'dir')
  return workspace
}

/**
 * Tear the fixture workspace down on an interrupt.
 *
 * `finally` never runs for SIGINT/SIGTERM, which is exactly how the old
 * in-`src/` fixtures survived a Ctrl-C. The checkout is now safe either way;
 * this only stops an aborted run from leaving a symlinked `node_modules` tree
 * in the temp directory.
 */
function registerInterruptCleanup(workspace: string): void {
  const tempRoot = path.dirname(workspace)
  for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) {
    process.on(signal, () => {
      fs.rmSync(tempRoot, { recursive: true, force: true })
      process.exit(signal === 'SIGINT' ? 130 : 143)
    })
  }
}

/**
 * Minimal module body so an imported fixture target actually resolves.
 *
 * The stub re-exports whatever names the fixture imports, otherwise the
 * fixture workspace would not type-check and `tsPreCompilationDeps` would be
 * operating on a broken program.
 */
function stubModuleSource(filePath: string, exportedNames: readonly string[]): string {
  if (filePath.endsWith('.vue')) {
    return [
      '<script lang="ts">',
      "export default { name: 'BoundaryFixtureStub' }",
      `const { ${exportedNames.join(', ')} } = {} as Record<string, unknown>`,
      `export { ${exportedNames.join(', ')} }`,
      '</script>',
    ].join('\n')
  }
  const declarations = exportedNames.map(
    (name) => `export const ${name} = undefined as unknown as (() => void) & Record<string, never>`,
  )
  return [
    '// Generated by checkDependencyBoundaries: exists only so the fixture import',
    '// resolves into a real dependency edge for dependency-cruiser to judge.',
    'export default undefined as never',
    ...declarations,
  ].join('\n')
}

/** Names bound by `import { a, b as c } from '…'` in a fixture module. */
function importedNames(source: string, specifier: string): string[] {
  const names: string[] = []
  const pattern = new RegExp(
    `(?:import|export)\\s+(?:type\\s+)?\\{([^}]*)\\}\\s+from\\s*['"]${specifier.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}['"]`,
    'g',
  )
  for (const match of source.matchAll(pattern)) {
    for (const raw of (match[1] ?? '').split(',')) {
      const bound = raw
        .trim()
        .replace(/^type\s+/, '')
        .split(/\s+as\s+/)
        .at(-1)
        ?.trim()
      if (bound && /^[A-Za-z_$][\w$]*$/.test(bound)) names.push(bound)
    }
  }
  return names
}

function materializeFixture(workspace: string, file: FixtureFile): string {
  const fixtureAbsolute = path.join(workspace, file.filePath)
  fs.mkdirSync(path.dirname(fixtureAbsolute), { recursive: true })

  const targets = new Map<string, { specifier: string; absolute: string }>()
  const content = file.content.replace(/\{target:([^}]+)\}/g, (_match, targetModule: string) => {
    const existing = targets.get(targetModule)
    if (existing) return existing.specifier

    const targetWithExtension = /\.[a-z]+$/.test(targetModule) ? targetModule : `${targetModule}.ts`
    const targetAbsolute = path.join(workspace, targetWithExtension)

    // Extensionless specifiers match how the real modules are imported; the stub
    // keeps its extension so the file name stays readable in the workspace.
    const specifier = toPosix(path.relative(path.dirname(fixtureAbsolute), targetAbsolute))
    const withoutExtension = targetWithExtension.endsWith('.vue')
      ? specifier
      : specifier.replace(/\.ts$/, '')
    targets.set(targetModule, { specifier: withoutExtension, absolute: targetAbsolute })
    return withoutExtension
  })

  for (const { absolute, specifier } of targets.values()) {
    fs.mkdirSync(path.dirname(absolute), { recursive: true })
    fs.writeFileSync(
      absolute,
      stubModuleSource(path.relative(workspace, absolute), importedNames(content, specifier)),
    )
  }

  fs.writeFileSync(fixtureAbsolute, content)
  return toPosix(path.relative(workspace, fixtureAbsolute))
}

console.log('--- 1. Running dependency-cruiser on production codebase ---')

let inventorySeams: SeamRecord[]
try {
  inventorySeams = loadSeamInventory().seams
} catch (error) {
  console.error('Seam inventory failed to load; fix docs/frontend-seam-inventory.md first.')
  console.error(error instanceof Error ? error.message : error)
  process.exit(1)
}

const prodResult = runDepcruise(webRoot, [
  '--config',
  '.dependency-cruiser.cjs',
  '--output-type',
  'json',
  'src',
])
const prodOutput = JSON.parse(prodResult.stdout || '{}') as {
  summary?: { violations?: Array<{ from: string; to: string; rule?: { name?: string } }> }
  modules?: CruiseModule[]
}
const cruiseViolations = prodOutput.summary?.violations ?? []
const scannedBoardEdges = scanBoardDeepImports(webRoot)

/**
 * Stable-sorted per-seam fan-in/out JSON report.
 *
 * Built from the same depcruise pass that gates production, so the report
 * can never describe a different graph than the one that passed/failed.
 * Per-seam deep imports include every external edge into private ownership,
 * including tests and nested Seams; strict acceptance requires these arrays
 * to be empty alongside the dependency-cruiser rules and Board scanner.
 */
async function writeFanReport(): Promise<void> {
  const edges = graphEdges(prodOutput.modules ?? [])
  const publicEntries = new Set(
    inventorySeams.filter((seam) => seam.kind === 'canonical').map((seam) => seam.path),
  )
  const deepViolations = cruiseViolations.filter((violation) =>
    (violation.rule?.name ?? '').includes('deep'),
  )
  const shims = listShims(webRoot)
  const seams: Record<string, unknown> = {}
  for (const seam of inventorySeams) {
    const measured = measureSeam(seam, edges, publicEntries)
    const actualTargets = new Set(measured.fanOutTargets)
    const targetTargets = new Set(seam.targetFanOutTargets)
    const advisories = {
      fanInTargetMismatch: measured.fanIn !== seam.targetFanIn,
      fanOutTargetMismatch: measured.fanOut !== seam.targetFanOut,
      newFanOutTargets: measured.fanOutTargets.filter((target) => !targetTargets.has(target)),
      missingFanOutTargets: seam.targetFanOutTargets.filter((target) => !actualTargets.has(target)),
    }
    seams[seam.path] = {
      ...measured,
      targetFanIn: seam.targetFanIn,
      targetFanOut: seam.targetFanOut,
      advisories,
      shimCount: shims.filter((shim) => isOwned(shim, seam.ownershipScope)).length,
      ownershipScope: seam.ownershipScope,
      impureRuntimePaths: seam.purity === 'pure' ? findImpureRuntimePaths(seam.path, edges) : [],
    }
  }
  const report = {
    version: 2,
    checkpointSha: checkpointSha(),
    scanScope:
      'all src modules including tests; external means outside ownershipScope; fan-out includes direct local/package/core dependencies, never package internals',
    inventoryGate: process.argv.includes('--strict')
      ? 'strict all Seams; all cruiser/Board errors remain blocking'
      : 'inventory advisory; existing cruiser/Board errors remain blocking',
    cruiserConfig: '.dependency-cruiser.cjs',
    totals: {
      seams: inventorySeams.length,
      violations: cruiseViolations.length,
      boardDeepImports: scannedBoardEdges.size,
      shims: shims.length,
    },
    shims,
    undeclaredDeepImports: [
      ...deepViolations.map(
        (violation) => `${violation.from}|${violation.to}#${violation.rule?.name ?? ''}`,
      ),
      ...[...scannedBoardEdges].map((edge) => `${edge}#board-deep-import-scan`),
    ].sort(),
    seams,
  }
  const advisoryDeepEdges = new Set(
    Object.values(seams).flatMap((value) =>
      (value as ReturnType<typeof measureSeam>).undeclaredDeepImports.map(edgeKey),
    ),
  )
  const reportPath = reportPathFromArgs()
  fs.writeFileSync(
    reportPath,
    await format(JSON.stringify(report), {
      ...(await resolveConfig(path.join(webRoot, 'package.json'))),
      parser: 'json',
    }),
  )
  console.log(`  fan-in/out report: ${path.relative(webRoot, reportPathFromArgs())}`)
  if (advisoryDeepEdges.size)
    console.warn(
      `  Seam inventory: ${advisoryDeepEdges.size} distinct undeclared deep edges; see report.`,
    )
  const impurePaths = Object.values(seams).flatMap(
    (value) => (value as { impureRuntimePaths: string[][] }).impureRuntimePaths,
  )
  if (impurePaths.length)
    console.warn(
      `  Pure-entry advisory: ${impurePaths.length} forbidden runtime dependency path(s).`,
    )
  if (process.argv.includes('--strict')) {
    const acceptedRecords = Object.values(seams)
    const hasMismatch = acceptedRecords.some((value) => {
      const record = value as ReturnType<typeof measureSeam> & {
        advisories: {
          fanInTargetMismatch: boolean
          fanOutTargetMismatch: boolean
          newFanOutTargets: string[]
          missingFanOutTargets: string[]
        }
      }
      return (
        record.undeclaredDeepImports.length ||
        record.undeclaredConsumers.length ||
        record.missingConsumers.length ||
        record.missingDeepImports.length ||
        record.advisories.fanInTargetMismatch ||
        record.advisories.fanOutTargetMismatch ||
        record.advisories.newFanOutTargets.length ||
        record.advisories.missingFanOutTargets.length
      )
    })
    const hasImpurePath = acceptedRecords.some(
      (value) => (value as { impureRuntimePaths: string[][] }).impureRuntimePaths.length,
    )
    if (hasMismatch || hasImpurePath || shims.length > 0) {
      console.error(
        'Strict Seam acceptance failed: frozen consumers/targets, purity, deep edges or shim inventory differ from the current contract.',
      )
      process.exit(1)
    }
  }
}
await writeFanReport()
if (prodResult.status !== 0 || cruiseViolations.length > 0 || scannedBoardEdges.size > 0) {
  console.error('Production dependency boundaries check failed!')
  console.error('Dependency-cruiser violations:', cruiseViolations)
  console.error('Board deep imports:', [...scannedBoardEdges])
  process.exit(1)
}

/**
 * Coverage report for the rule that had the widest blind spot.
 *
 * `route-pages-no-direct-generated-query` silently covered 13 of 31 route pages
 * while reading like a total ban, so the count is printed every run: a drop here
 * means the rule narrowed again.
 */
const ROUTE_PAGE_PATTERN = /^src\/app\/|src\/.*(?:Page|Layout)\.vue$/
function listRouteSurfaces(root: string): string[] {
  const found: string[] = []
  const visit = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      if (entry.name === 'node_modules' || entry.name === 'generated-query') continue
      const absolute = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        visit(absolute)
        continue
      }
      if (!/\.(?:vue|ts)$/.test(entry.name)) continue
      const relative = toPosix(path.relative(root, absolute))
      if (ROUTE_PAGE_PATTERN.test(relative)) found.push(relative)
    }
  }
  visit(path.join(root, 'src'))
  return found.sort()
}

const routeSurfaces = listRouteSurfaces(webRoot)
const namedRoutePages = routeSurfaces.filter((file) => /(?:Page|Layout)\.vue$/.test(file))
if (namedRoutePages.length === 0) {
  console.error('Production dependency boundaries check failed!')
  console.error('No *Page.vue / *Layout.vue found; the route-page rule regex is probably wrong.')
  process.exit(1)
}
console.log('✓ Production codebase satisfies all dependency boundary rules.')
console.log(
  `  route-pages-no-direct-generated-query covers ${namedRoutePages.length} route page(s) + ${routeSurfaces.length - namedRoutePages.length} app-layer module(s).`,
)
console.log('')

console.log('--- 2. Validating Positive & Negative Fixtures ---')
let hasError = false

const workspace = createFixtureWorkspace()
registerInterruptCleanup(workspace)
try {
  type CruiseModule = {
    source: string
    dependencies?: Array<{
      module: string
      resolved?: string
      couldNotResolve?: boolean
    }>
  }

  type CruiseViolation = {
    rule?: {
      name?: string
    }
  }

  // 2.1 Positive Fixture: Vue SFC + dynamic import resolution
  // Fixture files are NOT dot-prefixed: TypeScript's `include` globs skip
  // dot-files, so a workspace containing only dot-files makes `tsPreCompilationDeps`
  // fail with TS18003 and dependency-cruiser silently degrades to zero violations.
  const posDepPath = path.join(workspace, 'src/shared/pos-dep.fixture.ts')
  const posVuePath = path.join(workspace, 'src/shared/pos-vue.fixture.vue')
  fs.mkdirSync(path.dirname(posDepPath), { recursive: true })
  fs.writeFileSync(posDepPath, `export const lazyValue = 'ok';\n`)
  fs.writeFileSync(
    posVuePath,
    `<script setup lang="ts">\nconst load = () => import('./pos-dep.fixture');\n</script>`,
  )

  const posResult = runDepcruise(workspace, [
    '--config',
    '.dependency-cruiser.cjs',
    '--max-depth',
    '2',
    '--output-type',
    'json',
    toPosix(path.relative(workspace, posVuePath)),
  ])

  const posOutput = JSON.parse(posResult.stdout || '{}') as { modules?: CruiseModule[] }
  const posModule = posOutput.modules?.find((m) => m.source.includes('pos-vue.fixture.vue'))
  const posDep = posModule?.dependencies?.find((d) => d.module.includes('pos-dep.fixture'))

  if (!posDep || posDep.couldNotResolve) {
    console.error('✗ Positive fixture failed: Vue SFC / dynamic import failed to resolve.')
    hasError = true
  } else {
    console.log('✓ Positive fixture: Vue SFC and dynamic import resolved successfully.')
  }

  /**
   * Positive fixtures: dependency shapes that must stay legal.
   *
   * - Valid public entries: cross-feature imports through the declared seam
   *   (`board/index`, `auth/index`) — the §6.2 consumer-convergence shape.
   * - Pure subpaths: tool `core/` importing `localeCore` constants/types, and
   *   canvas importing the board `ports/canvas` port instead of the board root.
   * Each case expects zero violations of any boundary rule.
   */
  const positiveCases: Array<{ name: string; file: FixtureFile }> = [
    ...approvedFeatureShims.map((shim) => ({
      name: `allows the approved ${shim} compatibility shim`,
      file: {
        filePath: `src/shared/${shim}.shim.ts`,
        content: `export { value } from '{target:src/features/legal/index}';`,
      },
    })),
    ...['runtime', 'type-only'].map((kind) => ({
      name: `allows feature ${kind} dependency on core route policy`,
      file: {
        filePath: `src/features/legal/boundary-policy-${kind}.fixture.ts`,
        content:
          kind === 'runtime'
            ? `import { HOME_PATH } from '{target:src/core/auth/routePolicy}'; console.log(HOME_PATH);`
            : `import type { HOME_PATH } from '{target:src/core/auth/routePolicy}'; export type Probe = typeof HOME_PATH;`,
      },
    })),
    {
      name: 'allows horizontal modules to consume core',
      file: {
        filePath: 'src/api/boundary-core-ok.fixture.ts',
        content: `import { HOME_PATH } from '{target:src/core/auth/routePolicy}'; console.log(HOME_PATH);`,
      },
    },
    {
      name: 'allows consuming board through its public entrypoint',
      file: {
        filePath: 'src/features/admin/boundary-board-public-ok.fixture.ts',
        content: `import { loadBoardCatalog } from '{target:src/features/board/index}'; console.log(loadBoardCatalog);`,
      },
    },
    {
      name: 'allows consuming auth through its public entrypoint',
      file: {
        filePath: 'src/features/board/boundary-auth-public-ok.fixture.ts',
        content: `import { useAuthFeedback } from '{target:src/features/auth/index}'; console.log(useAuthFeedback);`,
      },
    },
    {
      name: 'allows tool core importing the pure localeCore subpath',
      file: {
        filePath: 'src/features/canvas/tools/retouch/core/boundary-locale-core.fixture.ts',
        content: `import { SUPPORTED_LOCALES } from '{target:src/core/i18n/localeCore}'; console.log(SUPPORTED_LOCALES);`,
      },
    },
    {
      name: 'allows spec files to deep-import retouch internals for mocks',
      file: {
        filePath: 'src/features/canvas/tools/image-tools/session/boundary-cross-tool-deep.spec.ts',
        content: `import { useRetouchSession } from '{target:src/features/canvas/tools/retouch/session/sessionTarget}'; console.log(useRetouchSession);`,
      },
    },
    {
      name: 'allows spec files to deep-import inpaint internals for mocks',
      file: {
        filePath: 'src/features/canvas/tools/image-tools/session/boundary-inpaint-deep.spec.ts',
        content: `import { useInpaintChromeState } from '{target:src/features/canvas/tools/inpaint/session/inpaintSessionTarget}'; console.log(useInpaintChromeState);`,
      },
    },
  ]

  for (const positive of positiveCases) {
    const positiveModule = materializeFixture(workspace, positive.file)
    const positiveResult = runDepcruise(workspace, [
      '--config',
      '.dependency-cruiser.cjs',
      '--output-type',
      'json',
      positiveModule,
    ])
    if (positiveResult.status !== 0) {
      console.error(`✗ Positive fixture failed: ${positive.name}.`)
      hasError = true
    } else {
      console.log(`✓ Positive fixture: ${positive.name}.`)
    }
  }

  for (const i18nRoot of ['shared', 'core']) {
    const pure = 'src/core/contracts/generationContract.ts'
    const bridge = 'src/core/contracts/bridge.ts'
    const runtime = `src/${i18nRoot}/i18n/i18nRuntime.ts`
    const pureAbsolute = path.join(workspace, pure)
    fs.mkdirSync(path.dirname(pureAbsolute), { recursive: true })
    fs.mkdirSync(path.dirname(path.join(workspace, runtime)), { recursive: true })
    fs.writeFileSync(path.join(workspace, runtime), 'export const i18n = {}\n')
    fs.writeFileSync(
      path.join(workspace, bridge),
      `export { i18n } from '../../${i18nRoot}/i18n/i18nRuntime'\n`,
    )
    for (const [source, expected, label] of [
      ['export type Output = string\n', 0, 'pure type contract'],
      [
        `import type { i18n } from '../../${i18nRoot}/i18n/i18nRuntime'\nexport type Output = typeof i18n\n`,
        0,
        'type-only runtime reference',
      ],
      [
        `export { i18n } from '../../${i18nRoot}/i18n/i18nRuntime'\n`,
        1,
        'direct runtime inclusion',
      ],
      [`export { i18n } from './bridge'\n`, 1, 'transitive runtime inclusion'],
    ] as const) {
      fs.writeFileSync(pureAbsolute, source)
      const result = runDepcruise(workspace, [
        '--config',
        '.dependency-cruiser.cjs',
        '--output-type',
        'json',
        pure,
      ])
      const output = JSON.parse(result.stdout || '{}') as {
        modules?: import('./seamInventory.ts').CruiseModule[]
      }
      const actual = findImpureRuntimePaths(pure, graphEdges(output.modules ?? []))
      if (actual.length !== expected || !output.modules?.some((module) => module.source === pure)) {
        console.error(`✗ Pure-subpath fixture: ${i18nRoot} ${label}.`)
        hasError = true
      } else console.log(`✓ Pure-subpath fixture: ${i18nRoot} ${label}.`)
    }
  }

  // 2.2 Negative Fixtures: Rule violation detection
  for (const testCase of fixtureCases) {
    const entry = testCase.files[0]
    const rest = testCase.files.slice(1)
    if (!entry) {
      console.error(`✗ Negative fixture "${testCase.name}" declares no files.`)
      hasError = true
      continue
    }
    for (const file of rest) materializeFixture(workspace, file)
    const fixtureModule = materializeFixture(workspace, entry)

    const negResult = runDepcruise(workspace, [
      '--config',
      '.dependency-cruiser.cjs',
      '--max-depth',
      '2',
      '--output-type',
      'json',
      fixtureModule,
    ])
    const negOutput = JSON.parse(negResult.stdout || '{}') as {
      summary?: { violations?: CruiseViolation[] }
    }
    const triggered = negOutput.summary?.violations?.some(
      (v) => v.rule?.name === testCase.expectedRule,
    )
    const scannerTriggered = testCase.scannerOnly
      ? scanBoardDeepImports(workspace, { includeFixtures: true }).has(
          `${fixtureModule}|src/features/board/core/index.ts`,
        )
      : false

    if (!triggered && !scannerTriggered) {
      console.error(
        `✗ Negative fixture failed: "${testCase.name}" (${fixtureModule}) expected to violate "${testCase.expectedRule}", but did not.`,
      )
      console.error('Actual violations:', negOutput.summary?.violations)
      hasError = true
    } else {
      console.log(
        `✓ Negative fixture: "${testCase.name}" successfully caught violation "${testCase.expectedRule}".`,
      )
    }
  }
} finally {
  fs.rmSync(path.dirname(workspace), { recursive: true, force: true })
}

if (hasError) {
  process.exit(1)
}

console.log('\n✓ All architecture boundary positive and negative fixtures passed!')
