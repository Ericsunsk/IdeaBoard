import { describe, expect, it } from 'vitest'
import { SERVICE } from '../../../shared/domain/imageServices'
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
        slugs: [SERVICE.render, SERVICE.variations, SERVICE.inpaint, SERVICE.outpaint],
      },
      {
        key: 'prompt',
        title: 'Prompt',
        order: 20,
        slugs: [SERVICE.generation],
      },
      {
        key: 'fusion',
        title: 'Fusion',
        order: 30,
        slugs: [SERVICE.fusion],
      },
    ])
    expect(featureMap[SERVICE.render]).toEqual({
      slug: SERVICE.render,
      title: 'Image · 草图成图',
      label: '草图成图',
    })
  })
})
