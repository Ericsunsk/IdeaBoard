import { describe, expect, it } from 'vitest'
import {
  CANONICAL_WORKFLOW_FEATURE,
  CANONICAL_WORKFLOW_FEATURE_SLUGS,
  WORKFLOW_FEATURE_SLUGS,
} from './workflowFeatures'

describe('canonical workflow feature registry', () => {
  it('includes expand-phase identities without changing the production allowlist', () => {
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).toEqual(
      expect.arrayContaining([
        CANONICAL_WORKFLOW_FEATURE.promptGeneration,
        CANONICAL_WORKFLOW_FEATURE.imageEdit,
        CANONICAL_WORKFLOW_FEATURE.localInpaint,
      ]),
    )
    expect(WORKFLOW_FEATURE_SLUGS).not.toEqual(
      expect.arrayContaining([
        CANONICAL_WORKFLOW_FEATURE.promptGeneration,
        CANONICAL_WORKFLOW_FEATURE.imageEdit,
        CANONICAL_WORKFLOW_FEATURE.localInpaint,
      ]),
    )
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).not.toContain('partial-edit')
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).not.toContain('pattern-gen')
  })
})
