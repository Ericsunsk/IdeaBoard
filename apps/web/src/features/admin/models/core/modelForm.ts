import {
  isCloudflareHosted,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_THIRD_PARTY,
  type AdminModelItem,
} from './catalogData'
import * as v from 'valibot'
import type { DetectedGatewayModel } from '../resource'

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
// 为空即补收敛；capabilities 置空（未知不伪造），hosting 按供应商推导。
// 目录/探测不带价（后端票 773 已删 4 单价字段）：结算唯一依据 billing SKU UnitCredits，
// 参照价走 models-reference 接口按需查询，不随目录保存。descriptionEnriched 置 true：
// 导入文本都视为可被更新探测覆盖。
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
    hosting: isCloudflareHosted(model.provider)
      ? MODEL_HOSTING_CLOUDFLARE
      : MODEL_HOSTING_THIRD_PARTY,
    termsUrl: '',
    docsUrl: '',
    supportsPromptCaching: model.supports_prompt_caching,
  }
}

export function splitCapabilities(draft: string): string[] {
  return draft
    .split(',')
    .map((c) => c.trim())
    .filter(Boolean)
}

function isJsonParsable(input: string): boolean {
  try {
    JSON.parse(input)
    return true
  } catch {
    return false
  }
}

function isJsonObjectString(input: string): boolean {
  try {
    const parsed: unknown = JSON.parse(input)
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
  } catch {
    return false
  }
}

// 模型表单规则单源（schema 即真相）：Add 导入构造与 Edit 手工录入共用，调用方经标准 useForm 消费。
export const modelFormSchema = v.object({
  id: v.pipe(v.string(), v.trim(), v.minLength(1, '请输入模型标识 (ID)')),
  name: v.pipe(v.string(), v.trim(), v.minLength(1, '请输入模型显示名称')),
  provider: v.pipe(v.string(), v.trim(), v.minLength(1, '请输入提供商 (Provider)')),
  customParams: v.pipe(
    v.optional(v.string(), ''),
    v.trim(),
    v.check((value) => value === '' || isJsonParsable(value), '额外自定义参数 JSON 格式不合法'),
    v.check(
      (value) => value === '' || isJsonObjectString(value),
      '额外自定义参数必须是有效的 JSON 对象（键值对）',
    ),
  ),
})

// 提交归一：trim 全字段、无上下文回退默认分辨率、空能力保持空。
export function finalizeModelSubmit(form: AdminModelItem, capabilities: string[]): AdminModelItem {
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
