import { WORKFLOW_FEATURE, type WorkflowFeature } from './workflowFeatures'

export const WORKFLOW_FEATURE_GROUP = {
  prompt: 'prompt',
  display: 'display',
  fusion: 'fusion',
  inpaint: 'inpaint',
  imageTools: 'image-tools',
} as const

export type WorkflowFeatureGroupKey =
  (typeof WORKFLOW_FEATURE_GROUP)[keyof typeof WORKFLOW_FEATURE_GROUP]

export interface WorkflowFeatureGroupMeta {
  key: WorkflowFeatureGroupKey
  title: string
  desc: string
  order: number
}

export const WORKFLOW_FEATURE_GROUP_LIST: readonly WorkflowFeatureGroupMeta[] = [
  {
    key: WORKFLOW_FEATURE_GROUP.prompt,
    title: 'Prompt',
    desc: '草图成图、风格延展、抠图和图案生成类右键/生成工作流。',
    order: 10,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.display,
    title: 'Display',
    desc: '商品展示视角、光影和陈列表达。',
    order: 30,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.fusion,
    title: 'Fusion',
    desc: '2 到 4 图融合时的主次、权重和修饰语义。',
    order: 40,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.inpaint,
    title: 'Inpaint',
    desc: '局部重绘能力。',
    order: 50,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.imageTools,
    title: 'Image Tools',
    desc: '图片右键工具里的 HD 和扩图。',
    order: 60,
  },
] as const

export interface WorkflowFeatureRegistryItem {
  slug: WorkflowFeature
  label: string
  title: string
  adminTitle: string
  groupKey: WorkflowFeatureGroupKey
  imageActionLabel?: string
  imageActionTitle?: string
}

type WorkflowFeatureRegistryBaseItem = Omit<
  WorkflowFeatureRegistryItem,
  'title' | 'adminTitle'
>

const WORKFLOW_FEATURE_REGISTRY_BASE: readonly WorkflowFeatureRegistryBaseItem[] = [
  {
    slug: WORKFLOW_FEATURE.sketchToImage,
    label: '草图成图',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '草图成图',
    imageActionTitle: '草图成图',
  },
  {
    slug: WORKFLOW_FEATURE.partialEdit,
    label: '局部重绘',
    groupKey: WORKFLOW_FEATURE_GROUP.inpaint,
  },
  {
    slug: WORKFLOW_FEATURE.styleVariation,
    label: '风格延展',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '风格延展',
    imageActionTitle: '风格延展',
  },
  {
    slug: WORKFLOW_FEATURE.styleFusion,
    label: '多图融合',
    groupKey: WORKFLOW_FEATURE_GROUP.fusion,
  },
  {
    slug: WORKFLOW_FEATURE.virtual3d,
    label: '商品展示',
    groupKey: WORKFLOW_FEATURE_GROUP.display,
  },
  {
    slug: WORKFLOW_FEATURE.removeBg,
    label: '抠图',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '抠图',
    imageActionTitle: '抠图',
  },
  {
    slug: WORKFLOW_FEATURE.patternGen,
    label: '图案生成',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
  },
  {
    slug: WORKFLOW_FEATURE.hdUpscale,
    label: 'HD',
    groupKey: WORKFLOW_FEATURE_GROUP.imageTools,
    imageActionLabel: 'HD',
    imageActionTitle: 'HD',
  },
  {
    slug: WORKFLOW_FEATURE.imageExtend,
    label: '扩图',
    groupKey: WORKFLOW_FEATURE_GROUP.imageTools,
    imageActionLabel: '扩图',
    imageActionTitle: '扩图',
  },
] as const

function getWorkflowFeatureGroupTitle(groupKey: WorkflowFeatureGroupKey): string {
  return WORKFLOW_FEATURE_GROUP_LIST.find((group) => group.key === groupKey)?.title || groupKey
}

function getWorkflowFeatureTitle(item: WorkflowFeatureRegistryBaseItem): string {
  return `${getWorkflowFeatureGroupTitle(item.groupKey)} · ${item.label}`
}

export const WORKFLOW_FEATURE_REGISTRY: readonly WorkflowFeatureRegistryItem[] =
  WORKFLOW_FEATURE_REGISTRY_BASE.map((item) => {
    const title = getWorkflowFeatureTitle(item)
    return {
      ...item,
      title,
      adminTitle: title,
    }
  })

export const WORKFLOW_FEATURE_REGISTRY_MAP = Object.fromEntries(
  WORKFLOW_FEATURE_REGISTRY.map((item) => [item.slug, item]),
) as Record<WorkflowFeature, WorkflowFeatureRegistryItem>
