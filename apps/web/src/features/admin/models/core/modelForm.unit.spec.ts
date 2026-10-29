import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { isCloudflareHosted } from './catalogData'
import {
  blankModelForm,
  buildModelItemFromManual,
  finalizeModelSubmit,
  modelFormSchema,
  splitCapabilities,
} from './modelForm'

function firstSchemaMessage(input: unknown): string {
  const result = v.safeParse(modelFormSchema, input)
  if (result.success) return ''
  return result.issues[0]?.message ?? ''
}

describe('modelForm', () => {
  it('infers hosting from provider', () => {
    expect(isCloudflareHosted('Cloudflare Workers AI')).toBe(true)
    expect(isCloudflareHosted('fal')).toBe(false)
  })

  it('builds manual import items from provider slug and bare model id', () => {
    const item = buildModelItemFromManual(
      { slug: 'fal', display_name: 'Fal AI', id_prefix: 'fal_ai/' },
      'birefnet',
    )
    expect(item.id).toBe('birefnet')
    expect(item.name).toBe('birefnet')
    expect(item.provider).toBe('fal')
    expect(item.hosting).toBe('第三方')
    expect(item.capabilities).toEqual([])
    expect(item.description).toBe('')
    expect(item.descriptionEnriched).toBe(false)
  })

  it('validates required fields and custom JSON', () => {
    expect(firstSchemaMessage({ id: '', name: 'n', provider: 'p' })).toBe('请输入模型标识 (ID)')
    expect(firstSchemaMessage({ id: 'openai/gpt-4o', name: 'n', provider: 'openai' })).toBe(
      '模型标识不要重复所选提供商前缀',
    )
    expect(firstSchemaMessage({ id: 'm', name: 'n', provider: 'p', customParams: '{x' })).toBe(
      '额外自定义参数 JSON 格式不合法',
    )
    expect(firstSchemaMessage({ id: 'm', name: 'n', provider: 'p', customParams: '{"k":1}' })).toBe(
      '',
    )
  })

  it.each([
    ['openrouter', 'google/gemini-3-flash'],
    ['openrouter', 'openai/gpt-5-image'],
    ['fal', 'fal-ai/flux-pro/v1.1'],
  ])('accepts the native model name for %s', (provider, id) => {
    expect(v.safeParse(modelFormSchema, { id, name: id, provider }).success).toBe(true)
  })

  it('splits capabilities and keeps empty empty on submit', () => {
    expect(splitCapabilities('a, b,, ')).toEqual(['a', 'b'])
    const submitted = finalizeModelSubmit({ ...blankModelForm(), id: ' gpt-4o ' }, [])
    expect(submitted.id).toBe('gpt-4o')
    expect(submitted.capabilities).toEqual([])
    expect(submitted.contextOrResolution).toBe('2048×2048')
  })
})
