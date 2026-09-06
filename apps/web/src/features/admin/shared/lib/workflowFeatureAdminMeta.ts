import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'

const ADMIN_WORKFLOW_FEATURE_GROUP = {
  image: 'image',
  prompt: 'prompt',
  fusion: 'fusion',
} as const

export type AdminWorkflowFeatureGroupKey =
  (typeof ADMIN_WORKFLOW_FEATURE_GROUP)[keyof typeof ADMIN_WORKFLOW_FEATURE_GROUP]

export interface AdminWorkflowFeatureGroupMeta {
  key: AdminWorkflowFeatureGroupKey
  title: string
  order: number
}

export const ADMIN_WORKFLOW_FEATURE_GROUPS: readonly AdminWorkflowFeatureGroupMeta[] = [
  { key: ADMIN_WORKFLOW_FEATURE_GROUP.image, title: 'Image', order: 10 },
  { key: ADMIN_WORKFLOW_FEATURE_GROUP.prompt, title: 'Prompt', order: 20 },
  { key: ADMIN_WORKFLOW_FEATURE_GROUP.fusion, title: 'Fusion', order: 30 },
] as const

const ADMIN_WORKFLOW_FEATURE_GROUP_BY_KEY = Object.fromEntries(
  ADMIN_WORKFLOW_FEATURE_GROUPS.map((group) => [group.key, group]),
) as Readonly<Record<AdminWorkflowFeatureGroupKey, AdminWorkflowFeatureGroupMeta>>

export interface AdminWorkflowFeatureMeta {
  label: string
  groupKey: AdminWorkflowFeatureGroupKey
}

const ADMIN_WORKFLOW_FEATURE_META: Readonly<Record<WorkflowFeature, AdminWorkflowFeatureMeta>> = {
  [WORKFLOW_FEATURE.render]: { label: '草图成图', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image },
  [WORKFLOW_FEATURE.inpaint]: { label: '局部重绘', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image },
  [WORKFLOW_FEATURE.variations]: {
    label: '风格延展',
    groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image,
  },
  [WORKFLOW_FEATURE.fusion]: { label: '多图融合', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.fusion },
  [WORKFLOW_FEATURE.cutout]: { label: '抠图', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image },
  [WORKFLOW_FEATURE.generation]: {
    label: 'Prompt 生图',
    groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.prompt,
  },
  [WORKFLOW_FEATURE.upscale]: { label: 'Upscale', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image },
  [WORKFLOW_FEATURE.outpaint]: { label: '扩图', groupKey: ADMIN_WORKFLOW_FEATURE_GROUP.image },
}

export function getAdminWorkflowFeatureMeta(slug: string): AdminWorkflowFeatureMeta | null {
  return ADMIN_WORKFLOW_FEATURE_META[slug as WorkflowFeature] ?? null
}

export function getAdminWorkflowFeatureGroupMeta(
  groupKey: AdminWorkflowFeatureGroupKey,
): AdminWorkflowFeatureGroupMeta {
  return ADMIN_WORKFLOW_FEATURE_GROUP_BY_KEY[groupKey]
}
