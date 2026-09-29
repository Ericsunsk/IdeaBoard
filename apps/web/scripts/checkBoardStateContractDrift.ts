import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const files = {
  domain: path.join(webRoot, 'src/features/board/domain/document/nodes.ts'),
  schema: path.join(webRoot, 'src/features/board/domain/schema/stateSchema.ts'),
} as const

async function main() {
  // The legacy backend mirror (`apps/api/internal/boards/state.go`) was
  // removed when the old boards surface was retired. Board state validation
  // is now owned by the frontend Board domain; do not reintroduce a dead
  // backend source just to satisfy this guardrail.
  const [domain, schema] = await Promise.all([
    readFile(files.domain, 'utf8'),
    readFile(files.schema, 'utf8'),
  ])
  const violations: string[] = []
  const fields = ['id', 'nodeIds', 'x', 'y', 'width', 'height']

  if (!domain.includes('export interface CanvasGroup'))
    violations.push('CanvasGroup interface missing')
  for (const field of fields) {
    if (!new RegExp(`\\b${field}\\??:`).test(domain))
      violations.push(`CanvasGroup field missing: ${field}`)
  }
  if (!schema.includes('v.regex(/^group-\\d+$/)'))
    violations.push('frontend group id pattern drift')
  if (!schema.includes('v.regex(/^(?:image|prompt|fusion)-\\d+(?:\\.\\d+)?$/)')) {
    violations.push('frontend group node id pattern drift')
  }
  if (!schema.includes('positiveFiniteNumberSchema'))
    violations.push('frontend positive geometry guard missing')
  if (violations.length > 0) {
    console.error('BoardStateGroup contract drift check failed.')
    for (const violation of violations) console.error(`- ${violation}`)
    process.exitCode = 1
    return
  }
  console.log('BoardStateGroup contract drift check passed.')
}

main().catch((error: unknown) => {
  console.error('BoardStateGroup contract drift check could not inspect all contract sources.')
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
