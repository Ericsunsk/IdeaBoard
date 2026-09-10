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
}

const fixtureCases: BoundaryFixtureCase[] = [
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
    content: `import { useGetAdminMonitor } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'; console.log(useGetAdminMonitor);`,
  },
  {
    name: 'prohibits cross-feature deep imports into internal directories',
    expectedRule: 'no-cross-feature-deep-imports',
    fixtureFilePath: path.join(
      webRoot,
      'src/features/admin/.boundary-cross-feature-deep.fixture.ts',
    ),
    content: `import { useBoardRuntime } from '../canvas/workspace-runtime/internal/useBoardRuntime'; console.log(useBoardRuntime);`,
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
const prodResult = runDepcruise(['--config', '.dependency-cruiser.cjs', 'src'])
if (prodResult.status !== 0) {
  console.error('Production dependency boundaries check failed!')
  console.error(prodResult.stdout || prodResult.stderr)
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

    if (!triggered) {
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
