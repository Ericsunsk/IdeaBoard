import {
  isCloudflareHosted,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_THIRD_PARTY,
  type AdminModelItem,
} from './catalogData'
import type { DetectedGatewayModel } from '../infrastructure/adminGatewayResource'

// 模型表单规则单源：Add 导入构造与 Edit 手工录入共用校验、归一与 hosting 推导。
// 空能力即空数组，不伪造占位；预览占位文案只活在 Dialog 展示层。

export function blankModelForm(): AdminModelItem {
  return {
    id: '',
    name: '',
    provider: '',
    description: '',
    contextOrResolution: '2048×2048',
    capabilities: [],
    hosting: MODEL_HOSTING_CLOUDFLARE,
    termsUrl: '',
    docsUrl: '',
    descriptionEnriched: false,
  }
}

// 探测模型入库：description 为空即空（不再写占位文案），靠富化位 +
// 为空即补收敛；capabilities 置空（未知不伪造），hosting 按供应商推导，
// 社区参考价原样透传。descriptionEnriched 置 true：导入文本都视为可被更新探测覆盖。
export function buildModelItemFromDetected(model: DetectedGatewayModel): AdminModelItem {
  return {
    id: model.id,
    name: model.name || model.id,
    provider: model.provider,
    mode: model.mode,
    description: model.description?.trim() || '',
    descriptionEnriched: true,
    contextOrResolution: model.context_or_resolution || '',
    capabilities: [],
    hosting: isCloudflareHosted(model.provider) ? MODEL_HOSTING_CLOUDFLARE : MODEL_HOSTING_THIRD_PARTY,
    termsUrl: '',
    docsUrl: '',
    inputCostPerToken: model.input_cost_per_token,
    outputCostPerToken: model.output_cost_per_token,
    cacheReadCostPerToken: model.cache_read_input_token_cost,
    cacheCreateCostPerToken: model.cache_creation_input_token_cost,
    supportsPromptCaching: model.supports_prompt_caching,
  }
}

export function splitCapabilities(draft: string): string[] {
  return draft
    .split(',')
    .map((c) => c.trim())
    .filter(Boolean)
}

function parseAndValidateCustomParams(input: string): {
  valid: boolean
  error?: string
} {
  const trimmed = input.trim()
  if (!trimmed) return { valid: true }
  try {
    const parsed = JSON.parse(trimmed)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return { valid: false, error: '额外自定义参数必须是有效的 JSON 对象（键值对）' }
    }
    return { valid: true }
  } catch {
    return { valid: false, error: '额外自定义参数 JSON 格式不合法' }
  }
}

export interface ModelFormInput {
  id: string
  name: string
  provider: string
  customParams?: string | undefined
}

// 返回 '' 表示通过，否则为展示文案。
export function validateModelForm(input: ModelFormInput): string {
  if (!input.id.trim()) return '请输入模型标识 (ID)'
  if (!input.name.trim()) return '请输入模型显示名称'
  if (!input.provider.trim()) return '请输入提供商 (Provider)'
  const paramCheck = parseAndValidateCustomParams(input.customParams?.trim() || '')
  if (!paramCheck.valid) return paramCheck.error || ''
  return ''
}

// 提交归一：trim 全字段、无上下文回退默认分辨率、空能力保持空。
export function finalizeModelSubmit(
  form: AdminModelItem,
  capabilities: string[],
): AdminModelItem {
  return {
    ...form,
    id: form.id.trim(),
    name: form.name.trim(),
    provider: form.provider.trim(),
    mode: form.mode?.trim() || undefined,
    description: form.description.trim(),
    contextOrResolution: form.contextOrResolution.trim() || '2048×2048',
    capabilities,
    customPath: form.customPath?.trim() || '',
    customParams: form.customParams?.trim() || '',
  }
}
