import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE } from './workflowFeatures'
import {
  getWorkflowFeaturePromptVariables,
  getWorkflowFeatureImageActionText,
} from './workflowFeatureRegistry'

describe('workflowFeatureRegistry projections', () => {
  it('publishes prompt variables from the workflow code registry', () => {
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.variations)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{variation_strength}}', tone: 'control' },
    ])
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.styleFusion)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{fusion_input_weights}}', tone: 'control' },
    ])
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.cutout)).toEqual([])
  })

  it('hides user_prompt when image-fill providers receive it separately', () => {
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.inpaint)).toEqual([])
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.outpaint)).toEqual([])
  })

  it('exposes canvas image action text without feature modules rebuilding it', () => {
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.cutout)).toEqual({
      label: 'Cutout',
      title: 'Cutout',
    })
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.inpaint)).toEqual({
      label: 'Inpaint',
      title: 'Inpaint',
    })
  })
})
