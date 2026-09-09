import type { WorkflowFeature } from '../../../shared/domain/workflowFeatures'
import {
  ADMIN_WORKFLOW_FEATURE_GROUPS,
  getAdminWorkflowFeatureGroupMeta,
  getAdminWorkflowFeatureMeta,
  type AdminWorkflowFeatureGroupKey,
} from '../shared/lib/workflowFeatureAdminMeta'

export type FeatureSlug = WorkflowFeature
export interface FeatureItem {
  slug: FeatureSlug
  title: string
  label: string
}

export interface FeatureGroup {
  key: AdminWorkflowFeatureGroupKey
  title: string
  order: number
  items: readonly FeatureItem[]
}

const PROMPT_CONFIG_FEATURES_BY_GROUP: Readonly<
  Partial<Record<AdminWorkflowFeatureGroupKey, readonly FeatureSlug[]>>
> = {
  image: ['render', 'variations', 'inpaint', 'outpaint'],
  prompt: ['generation'],
  fusion: ['fusion'],
}

function toFeatureItem(slug: FeatureSlug): FeatureItem {
  const meta = getAdminWorkflowFeatureMeta(slug)
  if (!meta) throw new Error(`Unknown admin workflow feature: ${slug}`)
  const group = getAdminWorkflowFeatureGroupMeta(meta.groupKey)
  return {
    slug,
    title: `${group.title} · ${meta.label}`,
    label: meta.label,
  }
}

export const featureMap = Object.fromEntries(
  Object.values(PROMPT_CONFIG_FEATURES_BY_GROUP)
    .flat()
    .map((slug) => [slug, toFeatureItem(slug)]),
) as Partial<Record<FeatureSlug, FeatureItem>>

export const groupedFeatureGroups: readonly FeatureGroup[] = ADMIN_WORKFLOW_FEATURE_GROUPS.map(
  (group) => ({
    ...group,
    items: (PROMPT_CONFIG_FEATURES_BY_GROUP[group.key] ?? []).map((slug) => featureMap[slug]!),
  }),
)
