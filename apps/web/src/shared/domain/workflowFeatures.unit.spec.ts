import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE, WORKFLOW_FEATURE_SLUGS } from './workflowFeatures'

describe('workflow feature registry', () => {
  it('keeps the production allowlist on the clean-break English identities', () => {
    expect(WORKFLOW_FEATURE_SLUGS).toEqual(
      expect.arrayContaining([WORKFLOW_FEATURE.generation, WORKFLOW_FEATURE.inpaint]),
    )
    expect(WORKFLOW_FEATURE_SLUGS).not.toContain('local-inpaint')
    expect(WORKFLOW_FEATURE_SLUGS).not.toContain('image-extend')
    expect(WORKFLOW_FEATURE_SLUGS).not.toContain('pattern-gen')
  })
})
