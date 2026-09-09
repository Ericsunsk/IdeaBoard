import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE } from '../../../shared/domain/workflowFeatures'
import { featureMap, groupedFeatureGroups } from './featurePromptMeta'

describe('featurePromptMeta', () => {
  it('keeps prompt configuration labels and grouping within the Admin boundary', () => {
    expect(
      groupedFeatureGroups.map((group) => ({
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
          WORKFLOW_FEATURE.render,
          WORKFLOW_FEATURE.variations,
          WORKFLOW_FEATURE.inpaint,
          WORKFLOW_FEATURE.outpaint,
        ],
      },
      {
        key: 'prompt',
        title: 'Prompt',
        order: 20,
        slugs: [WORKFLOW_FEATURE.generation],
      },
      {
        key: 'fusion',
        title: 'Fusion',
        order: 30,
        slugs: [WORKFLOW_FEATURE.fusion],
      },
    ])
    expect(featureMap[WORKFLOW_FEATURE.render]).toEqual({
      slug: WORKFLOW_FEATURE.render,
      title: 'Image · 草图成图',
      label: '草图成图',
    })
  })
})
