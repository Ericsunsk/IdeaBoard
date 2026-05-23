import { WORKFLOW_FEATURE, type WorkflowFeature } from './workflowFeatures'

export const WORKFLOW_FEATURE_GROUP = {
  prompt: 'prompt',
  model: 'model',
  display: 'display',
  fusion: 'fusion',
  edit: 'edit',
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
    key: WORKFLOW_FEATURE_GROUP.model,
    title: 'Model',
    desc: '视角、姿态与主体跟随控制。',
    order: 20,
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
    desc: '双图融合时的主次、比例和修饰语义。',
    order: 40,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.edit,
    title: 'Edit',
    desc: '局部改款、换背景和定向换脸。',
    order: 50,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.imageTools,
    title: 'Image Tools',
    desc: '图片右键工具里的高清放大和图片延伸。',
    order: 60,
  },
] as const

export interface WorkflowFeatureRegistryItem {
  slug: WorkflowFeature
  adminTitle: string
  groupKey: WorkflowFeatureGroupKey
  imageActionLabel?: string
  imageActionTitle?: string
}

export const WORKFLOW_FEATURE_REGISTRY: readonly WorkflowFeatureRegistryItem[] = [
  {
    slug: WORKFLOW_FEATURE.sketchToImage,
    adminTitle: 'Quick · 草图成图',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '草图成图',
    imageActionTitle: '草图成图',
  },
  {
    slug: WORKFLOW_FEATURE.partialEdit,
    adminTitle: 'Edit · 局部改款',
    groupKey: WORKFLOW_FEATURE_GROUP.edit,
  },
  {
    slug: WORKFLOW_FEATURE.styleVariation,
    adminTitle: 'Quick · 风格延展',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '风格延展',
    imageActionTitle: '风格延展',
  },
  {
    slug: WORKFLOW_FEATURE.styleFusion,
    adminTitle: 'Fusion · 双图融合',
    groupKey: WORKFLOW_FEATURE_GROUP.fusion,
  },
  {
    slug: WORKFLOW_FEATURE.multiAngle,
    adminTitle: 'Model · 单视角控制',
    groupKey: WORKFLOW_FEATURE_GROUP.model,
  },
  {
    slug: WORKFLOW_FEATURE.virtual3d,
    adminTitle: 'Display · 商品展示',
    groupKey: WORKFLOW_FEATURE_GROUP.display,
  },
  {
    slug: WORKFLOW_FEATURE.faceSwap,
    adminTitle: 'Edit · 定向换脸',
    groupKey: WORKFLOW_FEATURE_GROUP.edit,
  },
  {
    slug: WORKFLOW_FEATURE.bgSwap,
    adminTitle: 'Edit · 换背景',
    groupKey: WORKFLOW_FEATURE_GROUP.edit,
  },
  {
    slug: WORKFLOW_FEATURE.removeBg,
    adminTitle: 'Quick · 抠图',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    imageActionLabel: '抠图',
    imageActionTitle: '抠图',
  },
  {
    slug: WORKFLOW_FEATURE.patternGen,
    adminTitle: 'Pattern · 图案生成',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
  },
  {
    slug: WORKFLOW_FEATURE.hdUpscale,
    adminTitle: 'Image Tools · 高清放大',
    groupKey: WORKFLOW_FEATURE_GROUP.imageTools,
    imageActionLabel: 'HD',
    imageActionTitle: 'HD',
  },
  {
    slug: WORKFLOW_FEATURE.imageExtend,
    adminTitle: 'Image Tools · 图片延伸',
    groupKey: WORKFLOW_FEATURE_GROUP.imageTools,
    imageActionLabel: '扩图',
    imageActionTitle: '扩图',
  },
] as const

export const WORKFLOW_FEATURE_REGISTRY_MAP = Object.fromEntries(
  WORKFLOW_FEATURE_REGISTRY.map((item) => [item.slug, item]),
) as Record<WorkflowFeature, WorkflowFeatureRegistryItem>
