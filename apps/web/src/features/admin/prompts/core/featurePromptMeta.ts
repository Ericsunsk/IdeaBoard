import type { Service } from '../../../../core/contracts/imageServices'
import {
  ADMIN_SERVICE_GROUPS,
  ADMIN_SERVICE_ORDER,
  getAdminServiceGroupMeta,
  getAdminServiceMeta,
  type AdminServiceGroupKey,
} from '../../shared'

export type FeatureSlug = Service
export type FeaturePromptStatus = 'configured' | 'unconfigured' | 'unsupported'

export function getFeaturePromptStatus(
  slug: FeatureSlug,
  isConfigured: boolean,
): FeaturePromptStatus {
  if (slug === 'cutout') return 'unsupported'
  return isConfigured ? 'configured' : 'unconfigured'
}

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
  ADMIN_SERVICE_ORDER.map((slug) => [slug, toFeatureItem(slug)]),
) as Partial<Record<FeatureSlug, FeatureItem>>

export const groupedFeatureGroups: readonly FeatureGroup[] = ADMIN_SERVICE_GROUPS.map((group) => ({
  ...group,
  items: ADMIN_SERVICE_ORDER.filter(
    (slug) => getAdminServiceMeta(slug)?.groupKey === group.key,
  ).map((slug) => featureMap[slug]!),
}))
