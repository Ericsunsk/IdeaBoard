import { describe, expect, it } from 'vitest'
import {
  CANONICAL_WORKFLOW_FEATURE,
  CANONICAL_WORKFLOW_FEATURE_SLUGS,
  WORKFLOW_FEATURE_SLUGS,
} from './workflowFeatures'

describe('canonical workflow feature registry', () => {
  it('uses the canonical clean-break identities as the production allowlist', () => {
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).toEqual(
      expect.arrayContaining([
        CANONICAL_WORKFLOW_FEATURE.promptGeneration,
        CANONICAL_WORKFLOW_FEATURE.partialEdit,
      ]),
    )
    expect(WORKFLOW_FEATURE_SLUGS).toEqual(CANONICAL_WORKFLOW_FEATURE_SLUGS)
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).not.toContain('local-inpaint')
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).not.toContain('image-extend')
    expect(CANONICAL_WORKFLOW_FEATURE_SLUGS).not.toContain('pattern-gen')
  })
})
