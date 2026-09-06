import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE_SLUGS } from './workflowFeatures'

/**
 * Contract test: the web slug registry and the backend OpenAPI enum are the
 * same set. Catches drift without either side owning the other.
 */
describe('workflow feature slug contract with the API', () => {
  const openapi = readFileSync(join(__dirname, '../../../../api/openapi.yaml'), 'utf8')

  function extractEnum(schemaName: string): string[] {
    const block = openapi.match(
      new RegExp(`${schemaName}:\\s*\\n(?:.*\\n)*?\\s+enum:\\n((?:\\s+- .*\\n?)+)`),
    )
    if (!block) throw new Error(`OpenAPI schema ${schemaName} not found`)
    return block[1]
      .split('\n')
      .map((line) => line.replace(/^\s+- /, '').trim())
      .filter(Boolean)
  }

  it('matches the WorkflowFeatureSlug enum exactly', () => {
    expect([...WORKFLOW_FEATURE_SLUGS].sort()).toEqual(extractEnum('WorkflowFeatureSlug').sort())
  })

  it('keeps both OpenAPI slug enums identical to each other', () => {
    expect(extractEnum('WorkflowFeatureSlug').sort()).toEqual(
      extractEnum('CanonicalWorkflowFeatureSlug').sort(),
    )
  })
})
