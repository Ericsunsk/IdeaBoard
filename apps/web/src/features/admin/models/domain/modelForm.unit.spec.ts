import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { isCloudflareHosted } from './catalogData'
import {
  blankModelForm,
  buildModelItemFromDetected,
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

  it('builds import items without fabricating capabilities', () => {
    const item = buildModelItemFromDetected({
      id: 'fal/birefnet',
      name: '',
      provider: 'Fal AI',
      mode: 'image_generation',
    })
    expect(item.name).toBe('fal/birefnet')
    expect(item.mode).toBe('image_generation')
    expect(item.hosting).toBe('第三方')
    expect(item.capabilities).toEqual([])
    expect(item.description).toBe('')
    expect(item.descriptionEnriched).toBe(true)
  })

  it('keeps only the prompt-caching capability bit on import (no pricing)', () => {
    const item = buildModelItemFromDetected({
      id: 'gpt-4o',
      name: 'GPT-4o',
      provider: 'OpenAI',
      supports_prompt_caching: true,
    })
    expect(item.supportsPromptCaching).toBe(true)
    expect(item).not.toHaveProperty('inputCostPerToken')
    expect(item).not.toHaveProperty('outputCostPerToken')
    expect(item).not.toHaveProperty('cacheReadCostPerToken')
    expect(item).not.toHaveProperty('cacheCreateCostPerToken')
  })

  it('validates required fields and custom JSON', () => {
    expect(firstSchemaMessage({ id: '', name: 'n', provider: 'p' })).toBe('请输入模型标识 (ID)')
    expect(firstSchemaMessage({ id: 'a/b', name: 'n', provider: 'p', customParams: '{x' })).toBe(
      '额外自定义参数 JSON 格式不合法',
    )
    expect(
      firstSchemaMessage({ id: 'a/b', name: 'n', provider: 'p', customParams: '{"k":1}' }),
    ).toBe('')
  })

  it('splits capabilities and keeps empty empty on submit', () => {
    expect(splitCapabilities('a, b,, ')).toEqual(['a', 'b'])
    const submitted = finalizeModelSubmit({ ...blankModelForm(), id: ' a/b ' }, [])
    expect(submitted.id).toBe('a/b')
    expect(submitted.capabilities).toEqual([])
    expect(submitted.contextOrResolution).toBe('2048×2048')
  })
})
