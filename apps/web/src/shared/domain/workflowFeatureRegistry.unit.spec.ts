import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE } from './workflowFeatures'
import {
  getWorkflowFeatureImageActionText,
  WORKFLOW_FEATURE_CONFIG_GROUPS,
  WORKFLOW_FEATURE_CONFIG_ITEM_MAP,
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
          WORKFLOW_FEATURE.removeBg,
          WORKFLOW_FEATURE.partialEdit,
          WORKFLOW_FEATURE.hdUpscale,
          WORKFLOW_FEATURE.imageExtend,
        ],
      },
      {
        key: 'prompt',
        title: 'Prompt',
        order: 20,
        slugs: [WORKFLOW_FEATURE.patternGen],
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
    expect(WORKFLOW_FEATURE_CONFIG_ITEM_MAP[WORKFLOW_FEATURE.patternGen]).toEqual({
      slug: WORKFLOW_FEATURE.patternGen,
      title: 'Prompt · 图案生成',
      label: '图案生成',
    })
  })

  it('exposes canvas image action text without feature modules rebuilding it', () => {
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.removeBg)).toEqual({
      label: '抠图',
      title: '抠图',
    })
    expect(getWorkflowFeatureImageActionText(WORKFLOW_FEATURE.partialEdit)).toEqual({
      label: 'Inpaint · 局部重绘',
      title: 'Inpaint · 局部重绘',
    })
  })
})
