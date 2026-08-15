import { WORKFLOW_FEATURE, type WorkflowFeature } from './workflowFeatures'

export const WORKFLOW_FEATURE_GROUP = {
  image: 'image',
  prompt: 'prompt',
  fusion: 'fusion',
} as const

export type WorkflowFeatureGroupKey =
  (typeof WORKFLOW_FEATURE_GROUP)[keyof typeof WORKFLOW_FEATURE_GROUP]

export interface WorkflowFeatureGroupMeta {
  key: WorkflowFeatureGroupKey
  title: string
  order: number
}

export const WORKFLOW_FEATURE_GROUP_LIST: readonly WorkflowFeatureGroupMeta[] = [
  {
    key: WORKFLOW_FEATURE_GROUP.image,
    title: 'Image',
    order: 10,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.prompt,
    title: 'Prompt',
    order: 20,
  },
  {
    key: WORKFLOW_FEATURE_GROUP.fusion,
    title: 'Fusion',
    order: 30,
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
  promptVariables: readonly WorkflowFeaturePromptVariable[]
}

export type WorkflowFeaturePromptVariableTone = 'prompt' | 'control' | 'context'

export interface WorkflowFeaturePromptVariable {
  key: `{{${string}}}`
  tone: WorkflowFeaturePromptVariableTone
}

export interface WorkflowFeatureConfigItem {
  slug: WorkflowFeature
  title: string
  label: string
}

export interface WorkflowFeatureConfigGroup {
  key: string
  title: string
  order: number
  items: readonly WorkflowFeatureConfigItem[]
}

export interface WorkflowFeatureImageActionText {
  label: string
  title: string
}

type WorkflowFeatureRegistryBaseItem = Omit<
  WorkflowFeatureRegistryItem,
  'title' | 'adminTitle' | 'promptVariables'
> & {
  promptVariables?: readonly WorkflowFeaturePromptVariable[]
}

const WORKFLOW_FEATURE_REGISTRY_BASE: readonly WorkflowFeatureRegistryBaseItem[] = [
  {
    slug: WORKFLOW_FEATURE.sketchToImage,
    label: '草图成图',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: '草图成图',
    imageActionTitle: '草图成图',
    promptVariables: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  },
  {
    slug: WORKFLOW_FEATURE.localInpaint,
    label: '局部重绘',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: '局部重绘',
    imageActionTitle: '局部重绘',
    promptVariables: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  },
  {
    slug: WORKFLOW_FEATURE.styleVariation,
    label: '风格延展',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: '风格延展',
    imageActionTitle: '风格延展',
    promptVariables: [
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{variation_strength}}', tone: 'control' },
    ],
  },
  {
    slug: WORKFLOW_FEATURE.styleFusion,
    label: '多图融合',
    groupKey: WORKFLOW_FEATURE_GROUP.fusion,
    promptVariables: [
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{fusion_input_weights}}', tone: 'control' },
    ],
  },
  {
    slug: WORKFLOW_FEATURE.removeBg,
    label: '抠图',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: '抠图',
    imageActionTitle: '抠图',
    promptVariables: [],
  },
  {
    slug: WORKFLOW_FEATURE.promptGeneration,
    label: 'Prompt 生图',
    groupKey: WORKFLOW_FEATURE_GROUP.prompt,
    promptVariables: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  },
  {
    slug: WORKFLOW_FEATURE.hdUpscale,
    label: 'HD',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: 'HD',
    imageActionTitle: 'HD',
    promptVariables: [],
  },
  {
    slug: WORKFLOW_FEATURE.imageExtend,
    label: '扩图',
    groupKey: WORKFLOW_FEATURE_GROUP.image,
    imageActionLabel: '扩图',
    imageActionTitle: '扩图',
    promptVariables: [
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{extend_top_percent}}', tone: 'control' },
      { key: '{{extend_right_percent}}', tone: 'control' },
      { key: '{{extend_bottom_percent}}', tone: 'control' },
      { key: '{{extend_left_percent}}', tone: 'control' },
    ],
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
      promptVariables: item.promptVariables ?? [],
    }
  })

export const WORKFLOW_FEATURE_REGISTRY_MAP = Object.fromEntries(
  WORKFLOW_FEATURE_REGISTRY.map((item) => [item.slug, item]),
) as Record<WorkflowFeature, WorkflowFeatureRegistryItem>

export function getWorkflowFeatureRegistryItem(slug: string): WorkflowFeatureRegistryItem | null {
  return WORKFLOW_FEATURE_REGISTRY.find((item) => item.slug === slug) ?? null
}

const WORKFLOW_FEATURE_CONFIG_GROUP_DEFS = [
  {
    key: 'image',
    title: 'Image',
    order: 10,
    slugs: [
      WORKFLOW_FEATURE.sketchToImage,
      WORKFLOW_FEATURE.styleVariation,
      WORKFLOW_FEATURE.localInpaint,
      WORKFLOW_FEATURE.imageExtend,
    ],
  },
  {
    key: 'prompt',
    title: 'Prompt',
    order: 20,
    slugs: [WORKFLOW_FEATURE.promptGeneration],
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
  slugs: readonly WorkflowFeature[]
}[]

function toWorkflowFeatureConfigItem(
  slug: WorkflowFeature,
  groupTitle: string,
): WorkflowFeatureConfigItem {
  const feature = WORKFLOW_FEATURE_REGISTRY_MAP[slug]
  return {
    slug,
    title: `${groupTitle} · ${feature.label}`,
    label: feature.label,
  }
}

export const WORKFLOW_FEATURE_CONFIG_ITEMS = WORKFLOW_FEATURE_CONFIG_GROUP_DEFS.flatMap((group) =>
  group.slugs.map((slug) => toWorkflowFeatureConfigItem(slug, group.title)),
)

export const WORKFLOW_FEATURE_CONFIG_ITEM_MAP = Object.fromEntries(
  WORKFLOW_FEATURE_CONFIG_ITEMS.map((item) => [item.slug, item]),
) as Record<WorkflowFeature, WorkflowFeatureConfigItem>

export const WORKFLOW_FEATURE_CONFIG_GROUPS = WORKFLOW_FEATURE_CONFIG_GROUP_DEFS.filter(
  (group) => group.slugs.length > 0,
)
  .sort((a, b) => a.order - b.order)
  .map((group) => ({
    key: group.key,
    title: group.title,
    order: group.order,
    items: group.slugs.map((slug) => WORKFLOW_FEATURE_CONFIG_ITEM_MAP[slug]),
  })) satisfies readonly WorkflowFeatureConfigGroup[]

export const WORKFLOW_FEATURE_IMAGE_ACTION_TEXT_MAP = Object.fromEntries(
  WORKFLOW_FEATURE_REGISTRY.map((item) => [
    item.slug,
    {
      label: item.imageActionLabel ?? item.adminTitle,
      title: item.imageActionTitle ?? item.imageActionLabel ?? item.adminTitle,
    },
  ]),
) as Record<WorkflowFeature, WorkflowFeatureImageActionText>

export function getWorkflowFeatureImageActionText(
  slug: WorkflowFeature,
): WorkflowFeatureImageActionText {
  return WORKFLOW_FEATURE_IMAGE_ACTION_TEXT_MAP[slug]
}

export function getWorkflowFeaturePromptVariables(
  slug: WorkflowFeature,
): readonly WorkflowFeaturePromptVariable[] {
  return WORKFLOW_FEATURE_REGISTRY_MAP[slug].promptVariables
}
