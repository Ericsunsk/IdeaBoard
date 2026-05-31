import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'

export interface VarDef {
  key: string
  desc: string
}

const promptVars: VarDef[] = [{ key: '{{user_prompt}}', desc: '当前请求拼装后的提示词文本' }]

const noVars: VarDef[] = []

type RegistryFeatureItem = {
  slug: WorkflowFeature
  title: string
}

export const featureList = [
  { slug: IMAGE_NODE_ACTION.sketchToImage, title: 'Image · 草图成图' },
  { slug: IMAGE_NODE_ACTION.styleVariation, title: 'Image · 风格延展' },
  { slug: IMAGE_NODE_ACTION.removeBg, title: 'Image · 抠图' },
  { slug: WORKFLOW_FEATURE.partialEdit, title: 'Image · 局部改款 / 局部重绘' },
  { slug: IMAGE_NODE_ACTION.hdUpscale, title: 'Image · HD 放大' },
  { slug: IMAGE_NODE_ACTION.imageExtend, title: 'Image · 图片延伸' },
  { slug: WORKFLOW_FEATURE.patternGen, title: 'Prompt · 纯文本生成（pattern-gen）' },
  { slug: WORKFLOW_FEATURE.styleFusion, title: 'Fusion · 图片融合' },
  { slug: WORKFLOW_FEATURE.virtual3d, title: 'Display · 商品展示' },
] as const satisfies readonly RegistryFeatureItem[]

export type FeatureSlug = RegistryFeatureItem['slug']
export type FeatureItem = RegistryFeatureItem

export const featureMap = Object.fromEntries(
  featureList.map((item) => [item.slug, item]),
) as Record<FeatureSlug, FeatureItem>

const featureGroupDefs = [
  {
    key: 'image',
    title: 'Image',
    desc: 'Image 节点的右键快捷处理、局部重绘、图片延伸和 HD 放大配置。',
    order: 10,
    slugs: [
      IMAGE_NODE_ACTION.sketchToImage,
      IMAGE_NODE_ACTION.styleVariation,
      IMAGE_NODE_ACTION.removeBg,
      WORKFLOW_FEATURE.partialEdit,
      IMAGE_NODE_ACTION.hdUpscale,
      IMAGE_NODE_ACTION.imageExtend,
    ],
  },
  {
    key: 'prompt',
    title: 'Prompt',
    desc: 'Prompt 节点负责文本输入与纯文本生成配置。',
    order: 20,
    slugs: [WORKFLOW_FEATURE.patternGen],
  },
  {
    key: 'fusion',
    title: 'Fusion',
    desc: 'Fusion 节点的多图融合提示词、主次比例和融合语义配置。',
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
  [IMAGE_NODE_ACTION.sketchToImage]: noVars,
  [WORKFLOW_FEATURE.partialEdit]: promptVars,
  [IMAGE_NODE_ACTION.styleVariation]: [
    { key: '{{strength}}', desc: '风格延展强度百分比（0-100）' },
  ],
  [WORKFLOW_FEATURE.virtual3d]: [
    {
      key: '{{viewpoint}}',
      desc: 'Display 节点所选展示视角，系统会自动转换成中文描述（如：45° 侧视）',
    },
    { key: '{{lighting}}', desc: 'Display 节点光影，系统会自动转换成中文描述（如：高级暖白）' },
    {
      key: '{{user_prompt}}',
      desc: 'Prompt 节点输入口文本；未连接时系统会用当前视角/光影摘要兜底',
    },
  ],
  [IMAGE_NODE_ACTION.removeBg]: noVars,
  [WORKFLOW_FEATURE.patternGen]: noVars,
  [WORKFLOW_FEATURE.styleFusion]: [
    {
      key: '{{user_prompt}}',
      desc: '用于弱修饰/辅助修饰融合方式、质感、细节、氛围；Image A / Image B 主次建议通过滑块控制',
    },
    { key: '{{blend_ratio}}', desc: 'Image A 占比（0-100）' },
    { key: '{{image_a_weight}}', desc: 'Image A 实际融合百分比' },
    { key: '{{image_b_weight}}', desc: 'Image B 实际融合百分比' },
  ],
  [IMAGE_NODE_ACTION.hdUpscale]: [
    { key: '{{original_image_url}}', desc: '当前 Image 工具输入的原图 URL（自动传入）' },
    { key: '{{upscale_mode}}', desc: 'HD 模式（base / enhancement）' },
    { key: '{{upscale_factor}}', desc: '放大倍数（2 / 3 / 4）' },
    { key: '{{output_format}}', desc: '输出格式（jpg / png）' },
    { key: '{{output_quality}}', desc: '输出质量（30-100）' },
  ],
  [IMAGE_NODE_ACTION.imageExtend]: promptVars,
}

const featureNotes: Partial<Record<FeatureSlug, string>> = {
  [IMAGE_NODE_ACTION.sketchToImage]:
    'Image 节点右键「草图成图」使用固定 system prompt，不使用 {{user_prompt}} 模板变量。',
  [IMAGE_NODE_ACTION.styleVariation]:
    'Image 节点右键「风格延展」使用固定 system prompt，只使用 {{strength}} 控制延展幅度百分比（0-100）。',
  [IMAGE_NODE_ACTION.removeBg]:
    'Image 节点右键「抠图」是快捷工具，不接收用户输入；请配置固定 system prompt，不要使用 {{user_prompt}} 模板变量。',
  [WORKFLOW_FEATURE.partialEdit]:
    '归属 Image 节点：覆盖 Image 节点 Prompt 生成、局部重绘/局部改款等以原图为输入的 partial-edit 任务。',
  [WORKFLOW_FEATURE.patternGen]:
    'Prompt 节点纯文本生成会携带 pattern-gen feature slug，并读取这里的 system prompt。',
  [WORKFLOW_FEATURE.styleFusion]:
    'Fusion 节点生成时会携带 style-fusion feature slug，可用变量描述辅助修饰、融合比例和主次权重。',
  [WORKFLOW_FEATURE.virtual3d]:
    'Display 节点生成时会携带 virtual-3d feature slug，可结合视角、光影和连接的 Prompt 文本做展示约束。',
  [IMAGE_NODE_ACTION.hdUpscale]: 'Image 节点 HD 工具使用这里的固定 system prompt 与放大参数。',
  [IMAGE_NODE_ACTION.imageExtend]:
    'Image 节点图片延伸工具会把扩图输入作为 {{user_prompt}}，未填写扩图 Prompt 时只使用固定 system prompt。',
}

const CONTROL_VAR_KEYS = new Set([
  '{{strength}}',
  '{{blend_ratio}}',
  '{{image_a_weight}}',
  '{{image_b_weight}}',
  '{{upscale_factor}}',
  '{{output_quality}}',
])

const ASSET_VAR_KEYS = new Set(['{{original_image_url}}'])

export function getVarsForSlug(slug: FeatureSlug): VarDef[] {
  return featureVarMap[slug] || noVars
}

export function getFeatureNote(slug: FeatureSlug): string {
  return featureNotes[slug] || ''
}

export type VarTone = 'prompt' | 'control' | 'asset' | 'context'

export function getVarTone(varKey: string): VarTone {
  if (varKey === '{{user_prompt}}') return 'prompt'
  if (CONTROL_VAR_KEYS.has(varKey)) return 'control'
  if (ASSET_VAR_KEYS.has(varKey)) return 'asset'
  return 'context'
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
