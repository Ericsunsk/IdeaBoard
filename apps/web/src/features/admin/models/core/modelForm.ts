import {
  isCloudflareHosted,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_THIRD_PARTY,
  type AdminModelItem,
} from './catalogData'
import * as v from 'valibot'
import type { GatewayProviderOption } from '../resource'

// 模型表单规则单源：Add 导入构造与 Edit 手工录入共用校验、归一与 hosting 推导。
// 空能力即空数组，不伪造占位；预览占位文案只活在 Dialog 展示层.

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

// 手填模型入库：provider 下拉（registry slug 真值）+ 手填裸 model name。
// description 为空即空；capabilities 置空（未知不伪造），hosting 按供应商推导。
// 结算唯一依据 billing SKU UnitCredits，不随目录保存任何价格。
// 原生模型名可含 vendor/model；仅拒绝重复的所选 provider/ 前缀。
export function buildModelItemFromManual(
  provider: GatewayProviderOption,
  modelId: string,
): AdminModelItem {
  const id = modelId.trim()
  return {
    ...blankModelForm(),
    id,
    name: id,
    provider: provider.slug,
    contextOrResolution: '',
    hosting: isCloudflareHosted(provider.slug)
      ? MODEL_HOSTING_CLOUDFLARE
      : MODEL_HOSTING_THIRD_PARTY,
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
export function hasModelProviderPrefix(provider: string, model: string): boolean {
  const slug = provider.trim().toLowerCase()
  return slug !== '' && model.trim().toLowerCase().startsWith(`${slug}/`)
}

// 原生模型名保留嵌套路径；供应商前缀由独立 provider 字段拥有。
export const modelFormSchema = v.pipe(
  v.object({
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
  }),
  v.forward(
    v.partialCheck(
      [['id'], ['provider']],
      ({ id, provider }) => !hasModelProviderPrefix(provider, id),
      '模型标识不要重复所选提供商前缀',
    ),
    ['id'],
  ),
)

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
