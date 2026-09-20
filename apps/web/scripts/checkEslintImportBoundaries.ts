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
  // NOTE: module-direction rules (generated-query, shared -> features) are
  // owned by dependency-cruiser; this smoke covers only the ESLint-owned
  // Overlay adapter conventions from eslint.config.js.
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
    filePath: path.join(
      webRoot,
      'src/features/canvas/presentation/pages/import-boundary.fixture.ts',
    ),
    code: `
      import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle/useCanvasCustomLayerOverlay'

      console.log(useCanvasCustomLayerOverlay)
    `,
  },
]

const allowedBoundaryCases: BoundaryCase[] = [
  {
    name: 'Canvas route page can use dialog adapters and the Canvas overlay public barrel',
    filePath: path.join(
      webRoot,
      'src/features/canvas/presentation/pages/import-boundary.fixture.ts',
    ),
    code: `
      import { useDialogOverlayLifecycle } from '../../../shared/composables/useDialogOverlayLifecycle'
      import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle'

      console.log(useDialogOverlayLifecycle, useCanvasCustomLayerOverlay)
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
