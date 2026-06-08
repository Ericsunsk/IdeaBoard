import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../../shared/domain/workflowFeatureRegistry'

export type VarTone = 'prompt' | 'control' | 'context'

interface VarDef {
  key: string
  tone: VarTone
}

const promptVars: readonly VarDef[] = [
  {
    key: '{{user_prompt}}',
    tone: 'prompt',
  },
]

const noVars: readonly VarDef[] = []

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
  {
    key: 'display',
    title: 'Display',
    order: 40,
    slugs: [WORKFLOW_FEATURE.virtual3d],
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

const featureVarMap: Partial<Record<FeatureSlug, readonly VarDef[]>> = {
  [WORKFLOW_FEATURE.sketchToImage]: noVars,
  [WORKFLOW_FEATURE.partialEdit]: promptVars,
  [WORKFLOW_FEATURE.styleVariation]: [{ key: '{{strength}}', tone: 'control' }],
  [WORKFLOW_FEATURE.virtual3d]: [
    {
      key: '{{viewpoint}}',
      tone: 'context',
    },
    {
      key: '{{lighting}}',
      tone: 'context',
    },
    {
      key: '{{user_prompt}}',
      tone: 'prompt',
    },
  ],
  [WORKFLOW_FEATURE.removeBg]: noVars,
  [WORKFLOW_FEATURE.patternGen]: promptVars,
  [WORKFLOW_FEATURE.styleFusion]: [
    {
      key: '{{user_prompt}}',
      tone: 'prompt',
    },
    {
      key: '{{image_weights}}',
      tone: 'control',
    },
  ],
  [WORKFLOW_FEATURE.hdUpscale]: [
    { key: '{{upscale_mode}}', tone: 'control' },
    { key: '{{upscale_factor}}', tone: 'control' },
    { key: '{{output_format}}', tone: 'control' },
    { key: '{{output_quality}}', tone: 'control' },
  ],
  [WORKFLOW_FEATURE.imageExtend]: [
    ...promptVars,
    { key: '{{extend_top}}', tone: 'control' },
    { key: '{{extend_right}}', tone: 'control' },
    { key: '{{extend_bottom}}', tone: 'control' },
    { key: '{{extend_left}}', tone: 'control' },
  ],
}

export function getVarsForSlug(slug: FeatureSlug): readonly VarDef[] {
  return featureVarMap[slug] || noVars
}

export function getGroupVariableCount(group: FeatureGroup): number {
  return new Set(group.items.flatMap((item) => getVarsForSlug(item.slug).map((v) => v.key))).size
}

export function getGroupFilledCount(
  group: FeatureGroup,
  savedPromptMap: Record<string, string>,
): number {
  return group.items.filter((item) => (savedPromptMap[item.slug] || '').trim().length > 0).length
}
