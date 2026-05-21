import { WEAR_TILE_MODE_TEMPLATE_DESC } from '../../workspace/components/nodes/config/nodeOptionConfig'

export interface VarDef {
  key: string
  desc: string
}

const promptVars: VarDef[] = [{ key: '{{user_prompt}}', desc: '当前请求拼装后的提示词文本' }]

const noVars: VarDef[] = []

export const featureList = [
  { slug: 'sketch-to-design', title: 'Quick · 线稿成款' },
  { slug: 'partial-edit', title: 'Edit · 局部改款' },
  { slug: 'style-innovate', title: 'Quick · 款式创新' },
  { slug: 'style-fusion', title: 'Fusion · 双图融合' },
  { slug: 'fabric-on-body', title: 'Wear · 上身贴合' },
  { slug: 'multi-angle', title: 'Model · 单视角控制' },
  { slug: 'virtual-3d', title: 'Display · 商品展示' },
  { slug: 'face-swap', title: 'Edit · 定向换脸' },
  { slug: 'bg-swap', title: 'Edit · 换背景' },
  { slug: 'remove-bg', title: 'Quick · 抠图' },
  { slug: 'pattern-gen', title: 'Pattern · 图案生成' },
  { slug: 'hd-upscale', title: 'Enhance · 高清放大' },
  { slug: 'canvas-extend', title: 'Enhance · 画布延伸' },
] as const

export type FeatureSlug = (typeof featureList)[number]['slug']
export type FeatureItem = (typeof featureList)[number]

export const featureMap = Object.fromEntries(
  featureList.map((item) => [item.slug, item]),
) as Record<FeatureSlug, FeatureItem>

const featureGroupDefs: Array<{ key: string; title: string; desc: string; slugs: FeatureSlug[] }> =
  [
    {
      key: 'prompt',
      title: 'Prompt',
      desc: '线稿成款、款式创新、抠图和图案生成类右键/生成工作流。',
      slugs: ['sketch-to-design', 'style-innovate', 'remove-bg', 'pattern-gen'],
    },
    {
      key: 'wear',
      title: 'Wear',
      desc: '面料贴合、上身效果和蒙版结合逻辑。',
      slugs: ['fabric-on-body'],
    },
    {
      key: 'model',
      title: 'Model',
      desc: '视角、姿态与主体跟随控制。',
      slugs: ['multi-angle'],
    },
    {
      key: 'display',
      title: 'Display',
      desc: '商品展示视角、光影和陈列表达。',
      slugs: ['virtual-3d'],
    },
    {
      key: 'fusion',
      title: 'Fusion',
      desc: '双图融合时的主次、比例和修饰语义。',
      slugs: ['style-fusion'],
    },
    {
      key: 'edit',
      title: 'Edit',
      desc: '局部改款、换背景和定向换脸。',
      slugs: ['partial-edit', 'bg-swap', 'face-swap'],
    },
    {
      key: 'enhance',
      title: 'Enhance',
      desc: '超分、局部增强和画布延伸。',
      slugs: ['hd-upscale', 'canvas-extend'],
    },
  ]

export const groupedFeatureGroups = featureGroupDefs.map((group) => ({
  ...group,
  items: group.slugs.map((slug) => featureMap[slug]),
}))

export type FeatureGroup = (typeof groupedFeatureGroups)[number]

const featureVarMap: Partial<Record<FeatureSlug, VarDef[]>> = {
  'sketch-to-design': noVars,
  'partial-edit': promptVars,
  'style-innovate': [{ key: '{{strength}}', desc: '款式创新强度百分比（0-100）' }],
  'fabric-on-body': [
    { key: '{{tile_mode}}', desc: WEAR_TILE_MODE_TEMPLATE_DESC },
    { key: '{{fabric_rotation}}', desc: 'Texture 方向角度（0-360）' },
    { key: '{{fabric_scale}}', desc: 'Texture 大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: 'Texture 融合强度百分比（0-100）' },
    {
      key: '{{original_image_url}}',
      desc: 'Body 原图 URL（由 Wear 节点 Image 输入自动传入，无需手填）',
    },
    {
      key: '{{mask_image_url}}',
      desc: 'Mask 节点输出上传后的 URL（连接 Mask 时自动传入；未连接时为空）',
    },
    { key: '{{use_mask}}', desc: '是否启用蒙版（连接 Mask 时自动为 true，未连接时自动为 false）' },
  ],
  'multi-angle': [
    { key: '{{angle_spec}}', desc: '视角控制参数（JSON，系统按它决定 yaw 与景别）' },
    { key: '{{model_url}}', desc: '画布里接到 Model 输入口的参考图（系统自动传值）' },
    { key: '{{model_strength}}', desc: 'Model 输入口权重（0-100，越高越贴近参考图主体）' },
    { key: '{{pose_url}}', desc: '画布里接到 Pose 输入口的姿态参考图（系统自动传值）' },
    { key: '{{pose_strength}}', desc: 'Pose 输入口权重（0-100，越高越跟随姿态）' },
    { key: '{{background}}', desc: 'Model 节点背景参数（original / studio）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点输入口文本（弱修饰，用于氛围和细节）' },
  ],
  'virtual-3d': [
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
  'face-swap': noVars,
  'bg-swap': [
    {
      key: '{{user_prompt}}',
      desc: 'Prompt 节点输入口文本（可选，未连接时系统会用默认背景切换描述）',
    },
  ],
  'remove-bg': noVars,
  'pattern-gen': noVars,
  'style-fusion': [
    {
      key: '{{user_prompt}}',
      desc: '用于弱修饰/辅助修饰融合方式、质感、细节、氛围；Image A / Image B 主次建议通过滑块控制',
    },
    { key: '{{blend_ratio}}', desc: 'Image A 占比（0-100）' },
    { key: '{{image_a_weight}}', desc: 'Image A 实际融合百分比' },
    { key: '{{image_b_weight}}', desc: 'Image B 实际融合百分比' },
  ],
  'hd-upscale': [
    { key: '{{use_mask}}', desc: '是否启用局部增强（连接 Mask 时自动为 true，否则为 false）' },
    { key: '{{original_image_url}}', desc: 'Enhance 节点输入的原图 URL（自动传入）' },
    { key: '{{mask_image_url}}', desc: 'Mask 节点输出上传后的 URL（仅连接 Mask 时自动传入）' },
    { key: '{{upscale_mode}}', desc: 'HD 模式（base / enhancement）' },
    { key: '{{upscale_factor}}', desc: '放大倍数（2 / 3 / 4）' },
    { key: '{{output_format}}', desc: '输出格式（jpg / png）' },
    { key: '{{output_quality}}', desc: '输出质量（30-100）' },
  ],
  'canvas-extend': promptVars,
}

const featureNotes: Partial<Record<FeatureSlug, string>> = {
  'sketch-to-design':
    '画布右键「线稿成款」仍需要配置固定 system prompt，只是不使用 {{user_prompt}} 模板变量。',
  'style-innovate':
    '画布右键「款式创新」仍需要配置固定 system prompt，只使用 {{strength}} 控制创新幅度百分比（0-100），不使用 {{user_prompt}} 模板变量。',
  'remove-bg':
    '画布右键「抠图」是快捷工具，不接收用户输入；必须配置固定 system prompt，并且不要使用 {{user_prompt}} 模板变量。',
  'pattern-gen': '底部输入栏「图案生成」建议保持后台 system prompt 为空，让用户输入自由发挥。',
  'hd-upscale':
    'Enhance · HD 无论是否连接 Mask 都统一走任务链并使用这里的系统提示词；连接 Mask 时 use_mask=true（并自动带入 mask 相关参数），未连接时 use_mask=false。',
}

const CONTROL_VAR_KEYS = new Set([
  '{{strength}}',
  '{{blend_ratio}}',
  '{{image_a_weight}}',
  '{{image_b_weight}}',
  '{{fabric_rotation}}',
  '{{fabric_scale}}',
  '{{fabric_opacity}}',
  '{{model_strength}}',
  '{{pose_strength}}',
  '{{upscale_factor}}',
  '{{output_quality}}',
])

const ASSET_VAR_KEYS = new Set([
  '{{original_image_url}}',
  '{{mask_image_url}}',
  '{{model_url}}',
  '{{pose_url}}',
  '{{use_mask}}',
])

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
