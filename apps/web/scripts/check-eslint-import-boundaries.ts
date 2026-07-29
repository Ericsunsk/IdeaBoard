import { ESLint } from 'eslint'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))

type BoundaryCase = {
  name: string
  filePath: string
  code: string
}

const restrictedBoundaryCases: BoundaryCase[] = [
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
  {
    name: 'route page cannot import the base overlay lifecycle directly',
    filePath: path.join(webRoot, 'src/features/admin/pages/import-boundary.fixture.ts'),
    code: `
      import { useOverlayInteractionLifecycle } from '../../../shared/composables/useOverlayInteractionLifecycle'

      console.log(useOverlayInteractionLifecycle)
    `,
  },
  {
    name: 'Canvas route page cannot deep-import its overlay implementation',
    filePath: path.join(webRoot, 'src/features/canvas/pages/import-boundary.fixture.ts'),
    code: `
      import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle/useCanvasCustomLayerOverlay'

      console.log(useCanvasCustomLayerOverlay)
    `,
  },
]

const allowedBoundaryCases: BoundaryCase[] = [
  {
    name: 'Canvas route page can use Prime adapters and the Canvas overlay public barrel',
    filePath: path.join(webRoot, 'src/features/canvas/pages/import-boundary.fixture.ts'),
    code: `
      import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
      import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle'

      console.log(usePrimeDialogOverlayLifecycle, useCanvasCustomLayerOverlay)
    `,
  },
]

const eslint = new ESLint({ cwd: webRoot })
let failed = false

for (const boundaryCase of restrictedBoundaryCases) {
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

for (const boundaryCase of allowedBoundaryCases) {
  const [result] = await eslint.lintText(boundaryCase.code, {
    filePath: boundaryCase.filePath,
  })
  const matched = result?.messages.some((message) => message.ruleId === 'no-restricted-imports')
  if (!matched) continue

  failed = true
  console.error(`Import-boundary smoke failed: ${boundaryCase.name}`)
  console.error(`Expected approved Overlay adapter imports for ${boundaryCase.filePath}`)
}

if (failed) {
  process.exitCode = 1
}
