import type { Service } from '../../../shared/domain/imageServices'
import {
  ADMIN_SERVICE_GROUPS,
  getAdminServiceGroupMeta,
  getAdminServiceMeta,
  type AdminServiceGroupKey,
} from '../shared/lib/serviceAdminMeta'

export type FeatureSlug = Service
export interface FeatureItem {
  slug: FeatureSlug
  title: string
  label: string
}

export interface FeatureGroup {
  key: AdminServiceGroupKey
  title: string
  order: number
  items: readonly FeatureItem[]
}

const PROMPT_CONFIG_FEATURES_BY_GROUP: Readonly<
  Partial<Record<AdminServiceGroupKey, readonly FeatureSlug[]>>
> = {
  image: ['render', 'variations', 'inpaint', 'outpaint', 'upscale'],
  prompt: ['generation'],
  fusion: ['fusion'],
}

function toFeatureItem(slug: FeatureSlug): FeatureItem {
  const meta = getAdminServiceMeta(slug)
  if (!meta) throw new Error(`Unknown admin service: ${slug}`)
  const group = getAdminServiceGroupMeta(meta.groupKey)
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

export const groupedFeatureGroups: readonly FeatureGroup[] = ADMIN_SERVICE_GROUPS.map((group) => ({
  ...group,
  items: (PROMPT_CONFIG_FEATURES_BY_GROUP[group.key] ?? []).map((slug) => featureMap[slug]!),
}))
