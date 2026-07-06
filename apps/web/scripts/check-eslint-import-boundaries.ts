import { ESLint } from 'eslint'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))

type BoundaryCase = {
  name: string
  filePath: string
  code: string
}

const boundaryCases: BoundaryCase[] = [
  {
    name: 'app layer cannot import generated-query directly',
    filePath: path.join(webRoot, 'src/app/stores/import-boundary.fixture.ts'),
    code: `
      import { getUserInfo } from '../../shared/api/generated-query/user/user'

      console.log(getUserInfo)
    `,
  },
  {
    name: 'feature page cannot import generated-query directly',
    filePath: path.join(webRoot, 'src/features/admin/pages/import-boundary.fixture.ts'),
    code: `
      import { useGetAdminMonitor } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'

      console.log(useGetAdminMonitor)
    `,
  },
  {
    name: 'shared cannot import feature modules',
    filePath: path.join(webRoot, 'src/shared/api/import-boundary.fixture.ts'),
    code: `
      import { adminRoutes } from '../../features/admin/app/adminRoutes'

      console.log(adminRoutes)
    `,
  },
]

const eslint = new ESLint({ cwd: webRoot })
let failed = false

for (const boundaryCase of boundaryCases) {
  const [result] = await eslint.lintText(boundaryCase.code, {
    filePath: boundaryCase.filePath,
  })
  const matched = result?.messages.some((message) => message.ruleId === 'no-restricted-imports')
  if (!matched) {
    failed = true
    console.error(`Import-boundary smoke failed: ${boundaryCase.name}`)
    console.error(`Expected no-restricted-imports for ${boundaryCase.filePath}`)
  }
}

if (failed) {
  process.exitCode = 1
}
