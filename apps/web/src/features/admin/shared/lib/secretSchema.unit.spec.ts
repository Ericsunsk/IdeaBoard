import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { secretField } from './secretSchema'

describe('admin secret field schema', () => {
  it('accepts blank input when not required', () => {
    const schema = v.object({ apiKey: secretField(false, '请输入 API Key') })

    expect(v.safeParse(schema, { apiKey: '' }).success).toBe(true)
    expect(v.safeParse(schema, { apiKey: '  ' }).success).toBe(true)
  })

  it('requires a trimmed non-empty value when required', () => {
    const schema = v.object({ apiKey: secretField(true, '请输入 API Key') })

    expect(v.safeParse(schema, { apiKey: '' }).success).toBe(false)
    expect(v.safeParse(schema, { apiKey: '   ' }).success).toBe(false)
    const parsed = v.safeParse(schema, { apiKey: 'sk-live' })
    expect(parsed.success).toBe(true)
    if (parsed.success) expect(parsed.output.apiKey).toBe('sk-live')
  })
})
