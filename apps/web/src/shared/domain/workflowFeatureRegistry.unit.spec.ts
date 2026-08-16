import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE } from './workflowFeatures'
import {
  getWorkflowFeaturePromptVariables,
  getWorkflowFeatureImageActionText,
  WORKFLOW_FEATURE_CONFIG_GROUPS,
  WORKFLOW_FEATURE_CONFIG_ITEM_MAP,
  WORKFLOW_FEATURE_GROUP,
} from './workflowFeatureRegistry'

describe('workflowFeatureRegistry projections', () => {
  it('exposes admin prompt config groups in the preserved order', () => {
    expect(
      WORKFLOW_FEATURE_CONFIG_GROUPS.map((group) => ({
        key: group.key,
        title: group.title,
        order: group.order,
        slugs: group.items.map((item) => item.slug),
      })),
    ).toEqual([
      {
        key: 'image',
        title: 'Image',
        order: 10,
        slugs: [
          WORKFLOW_FEATURE.sketchToImage,
          WORKFLOW_FEATURE.styleVariation,
          WORKFLOW_FEATURE.partialEdit,
          WORKFLOW_FEATURE.outpaint,
        ],
      },
      {
        key: 'prompt',
        title: 'Prompt',
        order: 20,
        slugs: [WORKFLOW_FEATURE.promptGeneration],
      },
      {
        key: 'fusion',
        title: 'Fusion',
        order: 30,
        slugs: [WORKFLOW_FEATURE.styleFusion],
      },
    ])
  })

  it('uses prompt config group titles for admin feature item titles', () => {
    expect(WORKFLOW_FEATURE_CONFIG_ITEM_MAP[WORKFLOW_FEATURE.sketchToImage]).toEqual({
      slug: WORKFLOW_FEATURE.sketchToImage,
      title: 'Image · 草图成图',
      label: '草图成图',
    })
    expect(WORKFLOW_FEATURE_CONFIG_ITEM_MAP[WORKFLOW_FEATURE.promptGeneration]).toEqual({
      slug: WORKFLOW_FEATURE.promptGeneration,
      title: 'Prompt · Prompt 生图',
      label: 'Prompt 生图',
    })
  })

  it('publishes prompt variables from the workflow code registry', () => {
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.styleVariation)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{variation_strength}}', tone: 'control' },
    ])
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.styleFusion)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{fusion_input_weights}}', tone: 'control' },
    ])
    expect(getWorkflowFeaturePromptVariables(WORKFLOW_FEATURE.removeBg)).toEqual([])
  })

  it('exposes user_prompt for every configurable Image feature', () => {
    const imageGroup = WORKFLOW_FEATURE_CONFIG_GROUPS.find(
      (group) => group.key === WORKFLOW_FEATURE_GROUP.image,
    )
    expect(imageGroup).toBeDefined()
    for (const item of imageGroup?.items ?? []) {
      expect(getWorkflowFeaturePromptVariables(item.slug)).toEqual(
        expect.arrayContaining([{ key: '{{user_prompt}}', tone: 'prompt' }]),
      )
    }
  })

  it('exposes canvas image action text without feature modules rebuilding it', () => {
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.removeBg)).toEqual({
      label: '抠图',
      title: '抠图',
    })
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.partialEdit)).toEqual({
      label: '局部重绘',
      title: '局部重绘',
    })
  })
})
