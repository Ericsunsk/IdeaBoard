import { describe, expect, it } from 'vitest'
import { resolveFormFieldState } from './formFieldContract'

// Single derivation for every FieldShell adapter: core IbFormField, Admin
// AdminFormField and auth AuthFieldShell (via IbFormField) must observe the
// same ids and describedby chain for the same inputs.
describe('resolveFormFieldState', () => {
  it('derives stable ids with no association when there is no error or hint', () => {
    const state = resolveFormFieldState({ fallbackId: 'ib-form-field-1' })

    expect(state.fieldId).toBe('ib-form-field-1')
    expect(state.invalid).toBe(false)
    expect(state.ariaInvalid).toBeUndefined()
    expect(state.errorId).toBeUndefined()
    expect(state.hintId).toBeUndefined()
    expect(state.describedbyId).toBeUndefined()
  })

  it('prefers an explicit fieldId over the generated fallback', () => {
    const state = resolveFormFieldState({ fieldId: 'username', fallbackId: 'ib-form-field-1' })

    expect(state.fieldId).toBe('username')
  })

  it('points describedby at the error when invalid, even with a hint present', () => {
    const state = resolveFormFieldState({
      fallbackId: 'admin-form-field-1',
      error: 'API Key 不能为空',
      hint: '请输入有效的密钥',
    })

    expect(state.invalid).toBe(true)
    expect(state.ariaInvalid).toBe(true)
    expect(state.errorId).toBe('admin-form-field-1-error')
    expect(state.describedbyId).toBe('admin-form-field-1-error')
  })

  it('falls back to the hint id when there is no error', () => {
    const state = resolveFormFieldState({
      fallbackId: 'admin-form-field-1',
      hint: '请输入有效的密钥',
    })

    expect(state.invalid).toBe(false)
    expect(state.ariaInvalid).toBeUndefined()
    expect(state.errorId).toBeUndefined()
    expect(state.hintId).toBe('admin-form-field-1-hint')
    expect(state.describedbyId).toBe('admin-form-field-1-hint')
  })

  it('treats an empty error string as valid', () => {
    const state = resolveFormFieldState({ fallbackId: 'field-1', error: '' })

    expect(state.invalid).toBe(false)
    expect(state.describedbyId).toBeUndefined()
  })
})
