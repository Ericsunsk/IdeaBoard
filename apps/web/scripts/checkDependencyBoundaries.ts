import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))

type BoundaryFixtureCase = {
  name: string
  expectedRule: string
  fixtureFilePath: string
  content: string
  additionalFiles?: Array<{ filePath: string; content: string }>
  scannerOnly?: boolean
}

function resolveSourceModule(sourcePath: string, specifier: string): string | null {
  if (!specifier.startsWith('.') && !specifier.startsWith('@/') && !specifier.startsWith('src/'))
    return null
  const candidate = specifier.startsWith('@/')
    ? path.join(webRoot, 'src', specifier.slice(2))
    : specifier.startsWith('src/')
      ? path.join(webRoot, specifier)
      : path.resolve(path.dirname(sourcePath), specifier)
  const candidates = [
    candidate,
    `${candidate}.ts`,
    `${candidate}.tsx`,
    `${candidate}.vue`,
    path.join(candidate, 'index.ts'),
  ]
  const resolved = candidates.find((file) => fs.existsSync(file) && fs.statSync(file).isFile())
  return resolved ? path.relative(webRoot, resolved).split(path.sep).join('/') : null
}

function scanBoardDeepImports(includeFixtures = false): Set<string> {
  const edges = new Set<string>()
  const sourceFiles = runDepcruise([
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
      (!includeFixtures && module.source.includes('.fixture.'))
    )
      continue
    const absoluteSource = path.join(webRoot, module.source)
    const content = fs.readFileSync(absoluteSource, 'utf8')
    const importPattern = /(?:from\s+|import\s*\(\s*)['"]([^'"]+)['"]/g
    for (const match of content.matchAll(importPattern)) {
      const specifier = match[1]
      if (!specifier) continue
      const target = resolveSourceModule(absoluteSource, specifier)
      if (
        !target ||
        !/^src\/features\/board\/(?:domain|application|infrastructure|presentation|testing)(?:\/|$)/.test(
          target,
        )
      )
        continue
      edges.add(`${module.source}|${target}`)
    }
  }
  return edges
}

const fixtureCases: BoundaryFixtureCase[] = [
  {
    name: 'prohibits admin features from importing the public generated client',
    expectedRule: 'features-admin-no-public-client',
    fixtureFilePath: path.join(webRoot, 'src/features/admin/.boundary-admin-public.fixture.ts'),
    content: `import { getMe } from '../../shared/api/generated-query/public/me/me'; console.log(getMe);`,
  },
  {
    name: 'prohibits non-admin features from importing the control generated client',
    expectedRule: 'non-admin-no-control-client',
    fixtureFilePath: path.join(
      webRoot,
      'src/features/board/.boundary-non-admin-control.fixture.ts',
    ),
    content: `import { listControlUsers } from '../../shared/api/generated-query/control/control-users/control-users'; console.log(listControlUsers);`,
  },
  {
    name: 'detects circular dependencies',
    expectedRule: 'no-circular',
    fixtureFilePath: path.join(webRoot, 'src/shared/.circular-a.fixture.ts'),
    content: `import { circularB } from './.circular-b.fixture'; export const circularA = () => circularB();`,
    additionalFiles: [
      {
        filePath: path.join(webRoot, 'src/shared/.circular-b.fixture.ts'),
        content: `import { circularA } from './.circular-a.fixture'; export const circularB = () => circularA();`,
      },
    ],
  },
  {
    name: 'prohibits shared modules from importing concrete feature modules',
    expectedRule: 'shared-not-to-features',
    fixtureFilePath: path.join(webRoot, 'src/shared/.boundary-shared-to-feature.fixture.ts'),
    content: `import { adminRoutes } from '../features/admin/app/routes'; console.log(adminRoutes);`,
  },
  {
    name: 'prohibits route pages from importing generated-query directly',
    expectedRule: 'route-pages-no-direct-generated-query',
    fixtureFilePath: path.join(
      webRoot,
      'src/features/admin/pages/.boundary-page-to-query.fixture.ts',
    ),
    content: `import { getGetControlMonitorUrl } from '../../../shared/api/generated-query/control/control-platform/control-platform'; console.log(getGetControlMonitorUrl);`,
  },
  {
    name: 'prohibits cross-feature deep imports into internal directories',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(
      webRoot,
      'src/features/admin/.boundary-cross-feature-deep.fixture.ts',
    ),
    content: `import { createEmptyBoardSnapshot } from '../board/domain'; console.log(createEmptyBoardSnapshot);`,
  },
  {
    name: 'prohibits aliased imports into Board internals',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(webRoot, 'src/features/admin/.boundary-board-alias.fixture.ts'),
    content: `import { createEmptyBoardSnapshot } from '@/features/board/domain'; console.log(createEmptyBoardSnapshot);`,
    scannerOnly: true,
  },
  {
    name: 'prohibits cross-feature deep imports into auth session internals',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(webRoot, 'src/features/admin/.boundary-auth-deep.fixture.ts'),
    content: `import { useUserStore } from '../auth/session/userSession'; console.log(useUserStore);`,
  },
  {
    name: 'prohibits cross-feature deep imports into workspace infrastructure',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(
      webRoot,
      'src/features/admin/.boundary-workspace-deep.fixture.ts',
    ),
    content: `import { getWorkspaceSelectionStore } from '../workspace/infrastructure/workspaceContext'; console.log(getWorkspaceSelectionStore);`,
  },
  {
    name: 'prohibits cross-feature deep imports into library components',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(webRoot, 'src/features/search/.boundary-library-deep.fixture.ts'),
    content: `import LibraryFloatingIsland from '../library/components/LibraryFloatingIsland.vue'; console.log(LibraryFloatingIsland);`,
  },
  {
    name: 'prohibits cross-feature deep imports into search presentation',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(webRoot, 'src/features/admin/.boundary-search-deep.fixture.ts'),
    content: `import { useSearchScreenRuntime } from '../search/presentation/useSearchScreenRuntime'; console.log(useSearchScreenRuntime);`,
  },
]

function runDepcruise(args: string[]) {
  const result = spawnSync('npx', ['depcruise', ...args], {
    cwd: webRoot,
    encoding: 'utf8',
    maxBuffer: 50 * 1024 * 1024,
  })
  return result
}

console.log('--- 1. Running dependency-cruiser on production codebase ---')
const prodResult = runDepcruise([
  '--config',
  '.dependency-cruiser.cjs',
  '--output-type',
  'json',
  'src',
])
const prodOutput = JSON.parse(prodResult.stdout || '{}') as {
  summary?: { violations?: Array<{ from: string; to: string; rule?: { name?: string } }> }
}
const cruiseViolations = prodOutput.summary?.violations ?? []
const scannedBoardEdges = scanBoardDeepImports()
if (prodResult.status !== 0 || cruiseViolations.length > 0 || scannedBoardEdges.size > 0) {
  console.error('Production dependency boundaries check failed!')
  console.error('Dependency-cruiser violations:', cruiseViolations)
  console.error('Board deep imports:', [...scannedBoardEdges])
  process.exit(1)
}
console.log('✓ Production codebase satisfies all dependency boundary rules.\n')

console.log('--- 2. Validating Positive & Negative Fixtures ---')
let hasError = false
const createdFiles: string[] = []

try {
  // 2.1 Positive Fixture: Vue SFC + dynamic import resolution
  const posVuePath = path.join(webRoot, 'src/shared/.pos-vue.fixture.vue')
  const posDepPath = path.join(webRoot, 'src/shared/.pos-dep.fixture.ts')
  createdFiles.push(posVuePath, posDepPath)

  fs.writeFileSync(posDepPath, `export const lazyValue = 'ok';\n`)
  fs.writeFileSync(
    posVuePath,
    `<script setup lang="ts">\nconst load = () => import('./.pos-dep.fixture');\n</script>`,
  )

  const posResult = runDepcruise([
    '--config',
    '.dependency-cruiser.cjs',
    '--max-depth',
    '2',
    '--output-type',
    'json',
    posVuePath,
  ])
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

  const posOutput = JSON.parse(posResult.stdout || '{}') as { modules?: CruiseModule[] }
  const posModule = posOutput.modules?.find((m) => m.source.includes('.pos-vue.fixture.vue'))
  const posDep = posModule?.dependencies?.find((d) => d.module.includes('.pos-dep.fixture'))

  if (!posDep || posDep.couldNotResolve) {
    console.error('✗ Positive fixture failed: Vue SFC / dynamic import failed to resolve.')
    hasError = true
  } else {
    console.log('✓ Positive fixture: Vue SFC and dynamic import resolved successfully.')
  }

  // 2.2 Negative Fixtures: Rule violation detection
  for (const testCase of fixtureCases) {
    createdFiles.push(testCase.fixtureFilePath)
    fs.writeFileSync(testCase.fixtureFilePath, testCase.content)

    if (testCase.additionalFiles) {
      for (const extra of testCase.additionalFiles) {
        createdFiles.push(extra.filePath)
        fs.writeFileSync(extra.filePath, extra.content)
      }
    }

    const negResult = runDepcruise([
      '--config',
      '.dependency-cruiser.cjs',
      '--max-depth',
      '2',
      '--output-type',
      'json',
      testCase.fixtureFilePath,
    ])
    const negOutput = JSON.parse(negResult.stdout || '{}') as {
      summary?: { violations?: CruiseViolation[] }
    }
    const triggered = negOutput.summary?.violations?.some(
      (v) => v.rule?.name === testCase.expectedRule,
    )
    const scannerTriggered = testCase.scannerOnly
      ? scanBoardDeepImports(true).has(
          `${path.relative(webRoot, testCase.fixtureFilePath).split(path.sep).join('/')}|src/features/board/domain/index.ts`,
        )
      : false

    if (!triggered && !scannerTriggered) {
      console.error(
        `✗ Negative fixture failed: "${testCase.name}" expected to violate "${testCase.expectedRule}", but did not.`,
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
  for (const file of createdFiles) {
    if (fs.existsSync(file)) {
      try {
        fs.unlinkSync(file)
      } catch (error) {
        console.warn(
          `⚠ Could not remove temporary dependency-boundary fixture ${path.relative(webRoot, file)}:`,
          error,
        )
      }
    }
  }
}

if (hasError) {
  process.exit(1)
}

console.log('\n✓ All architecture boundary positive and negative fixtures passed!')
