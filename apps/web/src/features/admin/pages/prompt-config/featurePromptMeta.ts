import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../../shared/domain/workflowFeatureRegistry'

type RegistryFeatureItem = {
  slug: WorkflowFeature
  title: string
  label: string
}

export type FeatureSlug = RegistryFeatureItem['slug']
type FeatureItem = RegistryFeatureItem

const featureGroupDefs = [
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
] as const satisfies readonly {
  key: string
  title: string
  order: number
  slugs: readonly FeatureSlug[]
}[]

function toFeatureItem(slug: FeatureSlug, groupTitle: string): FeatureItem {
  const feature = WORKFLOW_FEATURE_REGISTRY_MAP[slug]
  return {
    slug,
    title: `${groupTitle} · ${feature.label}`,
    label: feature.label,
  }
}

const featureList = featureGroupDefs.flatMap((group) =>
  group.slugs.map((slug) => toFeatureItem(slug, group.title)),
)

export const featureMap = Object.fromEntries(
  featureList.map((item) => [item.slug, item]),
) as Record<FeatureSlug, FeatureItem>

const sortedFeatureGroupDefs = featureGroupDefs
  .filter((group) => group.slugs.length > 0)
  .sort((a, b) => a.order - b.order)

export const groupedFeatureGroups = sortedFeatureGroupDefs.map((group) => ({
  key: group.key,
  title: group.title,
  items: group.slugs.map((slug) => featureMap[slug]),
}))

export type FeatureGroup = (typeof groupedFeatureGroups)[number]

export function getGroupFilledCount(
  group: FeatureGroup,
  savedPromptMap: Record<string, string>,
): number {
  return group.items.filter((item) => (savedPromptMap[item.slug] || '').trim().length > 0).length
}
