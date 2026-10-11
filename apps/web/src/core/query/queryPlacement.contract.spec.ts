// @vitest-environment node
import { ESLint } from 'eslint'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const webRoot = fileURLToPath(new URL('../../../', import.meta.url))
const eslint = new ESLint({
  cwd: webRoot,
  overrideConfig: {
    languageOptions: { parserOptions: { project: false } },
    rules: { 'governance/boolean-prefix': 'off' },
  },
})
const queryRule = 'governance/query-placement'

type PlacementFixture = { name: string; importer: string; code: string }

function buildImportFixture(name: string, importer: string, source: string): PlacementFixture {
  return { name, importer, code: `import * as query from '${source}'; console.log(query)` }
}

const allowedFixtures: PlacementFixture[] = [
  ...[
    ['account resource', 'src/features/account/session/credits/resource.ts'],
    ['admin resource', 'src/features/admin/statistics/session/resource.ts'],
    ['workspace selection', 'src/features/workspace/session/selection.ts'],
    ['shared singleton', 'src/shared/query/queryClient.ts'],
    ['P1 core singleton', 'src/core/query/queryClient.ts'],
    ['app plugin registration', 'src/app/main.ts'],
    ['Canvas provider spec', 'src/features/canvas/query.unit.spec.ts'],
    ['Canvas contract harness', 'src/features/canvas/query.contract.spec.ts'],
    ['Canvas component harness', 'src/features/canvas/query.component.spec.ts'],
    ['Canvas browser harness', 'src/features/canvas/query.browser.spec.ts'],
    ['Canvas testing helper', 'src/features/canvas/session/testing/query.ts'],
    ['Canvas testing entry', 'src/features/canvas/session/testing.ts'],
    ['shared test harness', 'src/test/query.ts'],
  ].map(([name, importer]) => buildImportFixture(name!, importer!, '@tanstack/vue-query')),
  buildImportFixture(
    'Canvas virtualization',
    'src/features/canvas/session/viewport.ts',
    '@tanstack/vue-virtual',
  ),
  buildImportFixture(
    'Search virtualization',
    'src/features/search/session/viewport.ts',
    '@tanstack/vue-virtual',
  ),
  buildImportFixture(
    'Canvas task result helper',
    'src/features/canvas/session/task.ts',
    '@/shared/query/taskResult',
  ),
  buildImportFixture(
    'Canvas credit events',
    'src/features/canvas/session/task.ts',
    '@/shared/query/userCreditsInvalidation',
  ),
  buildImportFixture(
    'P1 task result helper',
    'src/features/canvas/session/task.ts',
    '@/core/query/taskResult',
  ),
  {
    name: 'auth identity cleanup capability',
    importer: 'src/features/auth/session/transition.ts',
    code: "import { clearAuthScopedQueryCache as clear } from '../../../shared/query/queryClient'; clear()",
  },
  {
    name: 'P1 auth identity cleanup capability',
    importer: 'src/features/auth/session/transition.ts',
    code: "import { clearAuthScopedQueryCache } from '@/core/query/queryClient'; clearAuthScopedQueryCache()",
  },
]

const advisoryFixtures: PlacementFixture[] = [
  ...['board', 'search', 'library', 'auth', 'legal'].map((feature) =>
    buildImportFixture(
      `${feature} new cached state`,
      `src/features/${feature}/session/query.ts`,
      '@tanstack/vue-query',
    ),
  ),
  buildImportFixture(
    'Board shared Query client',
    'src/features/board/session/sync/boardSync.ts',
    '../../../../shared/query/queryClient',
  ),
  {
    name: 'auth cleanup import cannot also expose the client',
    importer: 'src/features/auth/session/transition.ts',
    code: "import { clearAuthScopedQueryCache, queryClient } from '@/shared/query/queryClient'; console.log(clearAuthScopedQueryCache, queryClient)",
  },
  {
    name: 'auth dynamic Query client',
    importer: 'src/features/auth/session/transition.ts',
    code: "void import('@/shared/query/queryClient')",
  },
]

const canvasFixtures: PlacementFixture[] = [
  ...[
    '@tanstack/vue-query',
    '@tanstack/vue-query/build/modern',
    '@tanstack/query-core',
    '@tanstack/query-core/build/modern',
    '@/shared/query/queryClient',
    '@/shared/query/queryDefaults',
    '@/shared/query',
    '@/shared/query/index.ts',
    '@/core/query/queryClient',
    '@/core/query',
    '../../../shared/query/queryClient',
    '../../../core/query/queryDefaults.ts',
  ].map((source) =>
    buildImportFixture(`Canvas rejects ${source}`, 'src/features/canvas/session/query.ts', source),
  ),
  {
    name: 'Canvas dynamic import',
    importer: 'src/features/canvas/session/query.ts',
    code: "void import('@tanstack/vue-query')",
  },
  {
    name: 'Canvas dynamic shared client',
    importer: 'src/features/canvas/session/query.ts',
    code: "void import('../../../shared/query/queryClient')",
  },
  {
    name: 'Canvas named reexport',
    importer: 'src/features/canvas/session/query.ts',
    code: "export { QueryClient } from '@tanstack/vue-query'",
  },
  {
    name: 'Canvas star reexport',
    importer: 'src/features/canvas/session/query.ts',
    code: "export * from '@/shared/query/queryClient'",
  },
  {
    name: 'Canvas type dependency',
    importer: 'src/features/canvas/session/query.ts',
    code: "import type { QueryClient } from '@tanstack/vue-query'; export type Client = QueryClient",
  },
  {
    name: 'Canvas inline type dependency',
    importer: 'src/features/canvas/session/query.ts',
    code: "export type Client = import('@tanstack/vue-query').QueryClient",
  },
  {
    name: 'Canvas type reexport',
    importer: 'src/features/canvas/session/query.ts',
    code: "export type { QueryClient } from '@tanstack/vue-query'",
  },
  {
    name: 'Canvas require type syntax',
    importer: 'src/features/canvas/session/query.ts',
    code: "import query = require('@tanstack/vue-query'); console.log(query)",
  },
  {
    name: 'Canvas Vue script',
    importer: 'src/features/canvas/ui/pages/QueryFixture.vue',
    code: '<script setup lang="ts">import { QueryClient } from "@tanstack/vue-query"; console.log(QueryClient)</script><template><div /></template>',
  },
]

async function lintFixture(fixture: PlacementFixture) {
  const [result] = await eslint.lintText(fixture.code, {
    filePath: path.join(webRoot, fixture.importer),
  })
  expect(result?.messages.filter((message) => message.fatal)).toEqual([])
  return result!.messages
}

describe('Query placement through the actual ESLint config', () => {
  test.each(allowedFixtures)('$name remains allowed', async (fixture) => {
    expect((await lintFixture(fixture)).filter((message) => message.ruleId === queryRule)).toEqual(
      [],
    )
  })

  test.each(advisoryFixtures)('$name is warn-only during P0', async (fixture) => {
    const messages = (await lintFixture(fixture)).filter((message) => message.ruleId === queryRule)
    expect(messages.map((message) => message.severity)).toEqual([1])
  })

  test.each(canvasFixtures)('$name is an error', async (fixture) => {
    const messages = (await lintFixture(fixture)).filter((message) => message.ruleId === queryRule)
    expect(messages.map((message) => message.severity)).toEqual([2])
  })

  test.each(['pages', 'app'])(
    'Canvas %s retains the Overlay error alongside Query',
    async (layer) => {
      const messages = await lintFixture({
        name: 'Overlay and Query coexist',
        importer: `src/features/canvas/ui/${layer}/query.ts`,
        code: "import { useOverlayInteractionLifecycle } from '@/core/composables/useOverlayInteractionLifecycle'; import { QueryClient } from '@tanstack/vue-query'; console.log(useOverlayInteractionLifecycle, QueryClient)",
      })
      expect(
        messages
          .filter((message) => message.ruleId === 'no-restricted-imports')
          .map((message) => message.severity),
      ).toEqual([2])
      expect(
        messages
          .filter((message) => message.ruleId === queryRule)
          .map((message) => message.severity),
      ).toEqual([2])
    },
  )
})
