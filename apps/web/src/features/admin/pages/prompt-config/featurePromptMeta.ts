import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../../shared/domain/workflowFeatureRegistry'

export type VarTone = 'prompt' | 'control' | 'context'

export interface VarDef {
  key: string
  desc: string
  tone: VarTone
}

const promptVars: VarDef[] = [
  {
    key: '{{user_prompt}}',
    desc: '当前节点或工具请求拼装后的用户提示词文本',
    tone: 'prompt',
  },
]

const noVars: VarDef[] = []

type RegistryFeatureItem = {
  slug: WorkflowFeature
  title: string
  label: string
}

export type FeatureSlug = RegistryFeatureItem['slug']
export type FeatureItem = RegistryFeatureItem

const featureGroupDefs = [
  {
    key: 'image',
    title: 'Image',
    desc: 'Image 节点的右键快捷处理、局部重绘、扩图和 HD 配置。',
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
    desc: 'Prompt 节点的纯文本生成配置；system prompt 可选，用于后续补充辅助约束。',
    order: 20,
    slugs: [WORKFLOW_FEATURE.patternGen],
  },
  {
    key: 'fusion',
    title: 'Fusion',
    desc: 'Fusion 节点的多图融合提示词、输入权重和融合语义配置。',
    order: 30,
    slugs: [WORKFLOW_FEATURE.styleFusion],
  },
  {
    key: 'display',
    title: 'Display',
    desc: 'Display 节点的商品展示视角、光影和陈列语义配置。',
    order: 40,
    slugs: [WORKFLOW_FEATURE.virtual3d],
  },
] as const satisfies readonly {
  key: string
  title: string
  desc: string
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

export const featureList = featureGroupDefs.flatMap((group) =>
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
  desc: group.desc,
  items: group.slugs.map((slug) => featureMap[slug]),
}))

export type FeatureGroup = (typeof groupedFeatureGroups)[number]

const featureVarMap: Partial<Record<FeatureSlug, VarDef[]>> = {
  [WORKFLOW_FEATURE.sketchToImage]: noVars,
  [WORKFLOW_FEATURE.partialEdit]: promptVars,
  [WORKFLOW_FEATURE.styleVariation]: [
    { key: '{{strength}}', desc: '风格延展强度百分比（0-100）', tone: 'control' },
  ],
  [WORKFLOW_FEATURE.virtual3d]: [
    {
      key: '{{viewpoint}}',
      desc: 'Display 节点所选展示视角，系统会自动转换成中文描述（如：45° 侧视）',
      tone: 'context',
    },
    {
      key: '{{lighting}}',
      desc: 'Display 节点光影，系统会自动转换成中文描述（如：高级暖白）',
      tone: 'context',
    },
    {
      key: '{{user_prompt}}',
      desc: '连接的 Prompt 文本；未连接 Prompt 时为空，视角/光影由 {{viewpoint}} / {{lighting}} 表达',
      tone: 'prompt',
    },
  ],
  [WORKFLOW_FEATURE.removeBg]: noVars,
  [WORKFLOW_FEATURE.patternGen]: promptVars,
  [WORKFLOW_FEATURE.styleFusion]: [
    {
      key: '{{user_prompt}}',
      desc: 'Fusion 节点输入框或连接的 Prompt 文本，用于补充融合方向、质感、细节和氛围',
      tone: 'prompt',
    },
    {
      key: '{{image_weights}}',
      desc: 'Fusion 根据 blend point 计算出的有序图片权重，支持 2-4 张输入图',
      tone: 'control',
    },
  ],
  [WORKFLOW_FEATURE.hdUpscale]: [
    { key: '{{upscale_mode}}', desc: 'HD 模式（base / enhancement）', tone: 'control' },
    { key: '{{upscale_factor}}', desc: '放大倍数（2 / 3 / 4）', tone: 'control' },
    { key: '{{output_format}}', desc: '输出格式（jpg / png）', tone: 'control' },
    { key: '{{output_quality}}', desc: '输出质量（30-100）', tone: 'control' },
  ],
  [WORKFLOW_FEATURE.imageExtend]: [
    ...promptVars,
    { key: '{{extend_top}}', desc: '基于原图高度向上扩展的百分比', tone: 'control' },
    { key: '{{extend_right}}', desc: '基于原图宽度向右扩展的百分比', tone: 'control' },
    { key: '{{extend_bottom}}', desc: '基于原图高度向下扩展的百分比', tone: 'control' },
    { key: '{{extend_left}}', desc: '基于原图宽度向左扩展的百分比', tone: 'control' },
  ],
}

const featureNotes: Partial<Record<FeatureSlug, string>> = {
  [WORKFLOW_FEATURE.sketchToImage]:
    'Image 节点右键「草图成图」使用固定 system prompt，不使用 {{user_prompt}} 模板变量。',
  [WORKFLOW_FEATURE.styleVariation]:
    'Image 节点右键「风格延展」使用固定 system prompt，只使用 {{strength}} 控制延展幅度百分比（0-100）。',
  [WORKFLOW_FEATURE.removeBg]:
    'Image 节点右键「抠图」是快捷工具，不接收用户输入；请配置固定 system prompt，不要使用 {{user_prompt}} 模板变量。',
  [WORKFLOW_FEATURE.partialEdit]:
    '覆盖 Image 节点 Prompt 生成、Prompt 节点带图输入、右键「局部重绘」等以原图为输入的 partial-edit 任务。',
  [WORKFLOW_FEATURE.patternGen]:
    'Prompt 节点纯文本生成会携带 pattern-gen feature slug；这里的 system prompt 可选，未配置时只使用 Prompt 节点文本。{{user_prompt}} 是 Prompt 节点文本。',
  [WORKFLOW_FEATURE.styleFusion]:
    'Fusion 节点生成时会携带 style-fusion feature slug；blend point 会被转换为 {{image_weights}}，旧的 A/B 权重变量已下线。',
  [WORKFLOW_FEATURE.virtual3d]:
    'Display 节点生成时会携带 virtual-3d feature slug，可结合视角、光影和连接的 Prompt 文本做展示约束。',
  [WORKFLOW_FEATURE.hdUpscale]: 'Image 节点右键「HD」使用这里的固定 system prompt 与整图高清参数；局部重绘请维护 partial-edit。',
  [WORKFLOW_FEATURE.imageExtend]:
    'Image 节点右键「扩图」会把扩图输入作为 {{user_prompt}}，未填写扩图 Prompt 时只使用固定 system prompt。',
}

export function getVarsForSlug(slug: FeatureSlug): VarDef[] {
  return featureVarMap[slug] || noVars
}

export function getFeatureNote(slug: FeatureSlug): string {
  return featureNotes[slug] || ''
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
