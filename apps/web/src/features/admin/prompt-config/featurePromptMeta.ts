import { IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'
import { WEAR_TILE_MODE_TEMPLATE_DESC } from '../../../shared/domain/wearTileModes'
import { WORKFLOW_FEATURE } from '../../../shared/domain/workflowFeatures'
import {
  WORKFLOW_FEATURE_GROUP_LIST,
  WORKFLOW_FEATURE_REGISTRY,
} from '../../../shared/domain/workflowFeatureRegistry'

export interface VarDef {
  key: string
  desc: string
}

const promptVars: VarDef[] = [{ key: '{{user_prompt}}', desc: '当前请求拼装后的提示词文本' }]

const noVars: VarDef[] = []

type RegistryFeatureItem = {
  slug: (typeof WORKFLOW_FEATURE_REGISTRY)[number]['slug']
  title: string
}

export const featureList: readonly RegistryFeatureItem[] = WORKFLOW_FEATURE_REGISTRY.map((item) => ({
  slug: item.slug,
  title: item.adminTitle,
}))

export type FeatureSlug = RegistryFeatureItem['slug']
export type FeatureItem = RegistryFeatureItem

export const featureMap = Object.fromEntries(
  featureList.map((item) => [item.slug, item]),
) as Record<FeatureSlug, FeatureItem>

const featureGroupDefs = WORKFLOW_FEATURE_GROUP_LIST
  .map((group) => ({
    key: group.key,
    title: group.title,
    desc: group.desc,
    order: group.order,
    slugs: WORKFLOW_FEATURE_REGISTRY.filter((item) => item.groupKey === group.key).map(
      (item) => item.slug as FeatureSlug,
    ),
  }))
  .filter((group) => group.slugs.length > 0)
  .sort((a, b) => a.order - b.order)

export const groupedFeatureGroups = featureGroupDefs.map((group) => ({
  key: group.key,
  title: group.title,
  desc: group.desc,
  items: group.slugs.map((slug) => featureMap[slug]),
}))

export type FeatureGroup = (typeof groupedFeatureGroups)[number]

const featureVarMap: Partial<Record<FeatureSlug, VarDef[]>> = {
  [IMAGE_NODE_ACTION.sketchToImage]: noVars,
  [WORKFLOW_FEATURE.partialEdit]: promptVars,
  [IMAGE_NODE_ACTION.styleVariation]: [{ key: '{{strength}}', desc: '风格延展强度百分比（0-100）' }],
  [WORKFLOW_FEATURE.fabricOnBody]: [
    { key: '{{tile_mode}}', desc: WEAR_TILE_MODE_TEMPLATE_DESC },
    { key: '{{fabric_rotation}}', desc: 'Texture 方向角度（0-360）' },
    { key: '{{fabric_scale}}', desc: 'Texture 大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: 'Texture 融合强度百分比（0-100）' },
    {
      key: '{{original_image_url}}',
      desc: 'Body 原图 URL（由 Wear 节点 Image 输入自动传入，无需手填）',
    },
  ],
  [WORKFLOW_FEATURE.multiAngle]: [
    { key: '{{angle_spec}}', desc: '视角控制参数（JSON，系统按它决定 yaw 与景别）' },
    { key: '{{model_url}}', desc: '画布里接到 Model 输入口的参考图（系统自动传值）' },
    { key: '{{model_strength}}', desc: 'Model 输入口权重（0-100，越高越贴近参考图主体）' },
    { key: '{{pose_url}}', desc: '画布里接到 Pose 输入口的姿态参考图（系统自动传值）' },
    { key: '{{pose_strength}}', desc: 'Pose 输入口权重（0-100，越高越跟随姿态）' },
    { key: '{{background}}', desc: 'Model 节点背景参数（original / studio）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点输入口文本（弱修饰，用于氛围和细节）' },
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
  [WORKFLOW_FEATURE.faceSwap]: noVars,
  [WORKFLOW_FEATURE.bgSwap]: [
    {
      key: '{{user_prompt}}',
      desc: 'Prompt 节点输入口文本（可选，未连接时系统会用默认背景切换描述）',
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
    '画布右键「草图成图」仍需要配置固定 system prompt，只是不使用 {{user_prompt}} 模板变量。',
  [IMAGE_NODE_ACTION.styleVariation]:
    '画布右键「风格延展」仍需要配置固定 system prompt，只使用 {{strength}} 控制延展幅度百分比（0-100），不使用 {{user_prompt}} 模板变量。',
  [IMAGE_NODE_ACTION.removeBg]:
    '画布右键「抠图」是快捷工具，不接收用户输入；必须配置固定 system prompt，并且不要使用 {{user_prompt}} 模板变量。',
  [WORKFLOW_FEATURE.patternGen]:
    '底部输入栏「图案生成」建议保持后台 system prompt 为空，让用户输入自由发挥。',
  [IMAGE_NODE_ACTION.hdUpscale]:
    'Image Tools · HD 走 Image 右键工具链，使用这里的固定系统提示词与放大参数。',
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
  '{{model_url}}',
  '{{pose_url}}',
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
