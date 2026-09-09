import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createAccountEditor } from './accountEditor'
import type { ProviderAccountStore } from './workspace'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './limits'
import type { AdminProviderConfigFeedback } from './useProviderConfigFeedback'
import type { ProviderAccountRecord, ProviderEndpointRecord } from './types'

function createEndpoint(overrides: Partial<ProviderEndpointRecord> = {}): ProviderEndpointRecord {
  return {
    id: 7,
    apiType: 'openai-compatible',
    protocol: 'model-api',
    capabilities: [],
    name: 'Endpoint Seven',
    baseUrl: 'https://example.com/v1',
    apiKeyConfigured: true,
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function createAccount(overrides: Partial<ProviderAccountRecord> = {}): ProviderAccountRecord {
  return {
    id: 11,
    name: 'Existing Key',
    providerEndpointId: 7,
    apiType: 'openai-compatible',
    protocol: 'model-api',
    capabilities: [],
    baseUrl: 'https://example.com/v1',
    models: [],
    secretConfigured: true,
    maxConcurrent: 3,
    currentConcurrent: 0,
    waitQueueCount: 0,
    runningStatus: '',
    failCount: 0,
    weight: 1,
    timeoutSeconds: 120,
    extraConfig: '',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function createHarness(accounts: ProviderAccountRecord[] = []) {
  const store: ProviderAccountStore = {
    accountSaveLoading: ref(false),
    detectModelsLoading: ref(false),
    accountLimits: ref(FALLBACK_ADMIN_ACCOUNT_LIMITS),
    resolveProviderEndpointTarget: () => createEndpoint(),
    resolveAccountTarget: () => null,
    findProviderEndpointById: () => createEndpoint(),
    getProviderEndpointAccounts: () => accounts,
    getProviderEndpointView: (endpoint) => ({
      id: endpoint.id,
      name: endpoint.name,
      baseUrl: endpoint.baseUrl,
      apiType: endpoint.apiType,
      protocol: endpoint.protocol,
      capabilities: endpoint.capabilities,
      apiTypeLabel: endpoint.apiType,
      protocolLabel: endpoint.protocol,
      apiKeyConfigured: endpoint.apiKeyConfigured,
      status: endpoint.status,
      commandTarget: { kind: 'providerEndpoint', id: endpoint.id } as const,
    }),
    saveAccount: vi.fn(async (command: { afterSaved?: () => void }) => {
      command.afterSaved?.()
    }),
    detectModels: vi.fn(async (command) => ({
      detectedValues: command.detectedValues,
      sessionCache: command.sessionCache,
      hasShownApiKeyHint: command.hasShownApiKeyHint,
    })),
  }
  const feedback = {
    accountFormInvalid: vi.fn(),
    accountSaved: vi.fn(),
  } as unknown as AdminProviderConfigFeedback
  const editor = createAccountEditor({ store, feedback })
  return { editor, store, feedback }
}

describe('accountEditor against a stub store', () => {
  it('opens create with a unique name derived from sibling accounts', () => {
    const { editor } = createHarness([createAccount({ name: 'Endpoint Seven APIKey' })])
    editor.openCreate({ kind: 'providerEndpoint', id: 7 })

    expect(editor.visible.value).toBe(true)
    expect(editor.isEditing.value).toBe(false)
    expect(editor.form.value.name).toBe('Endpoint Seven APIKey 2')
  })

  it('rejects a duplicate account name without touching the store', async () => {
    const { editor, store, feedback } = createHarness([createAccount({ name: 'Dup Key' })])
    editor.openCreate({ kind: 'providerEndpoint', id: 7 })
    editor.form.value.name = 'dup key'
    editor.form.value.secret = 'sk-test'

    await editor.save()

    expect(editor.errors.value.name).toBe('同一端点下已存在同名 APIKey')
    expect(feedback.accountFormInvalid).toHaveBeenCalledTimes(1)
    expect(store.saveAccount).not.toHaveBeenCalled()
    expect(editor.visible.value).toBe(true)
  })

  it('requires a secret for new accounts', async () => {
    const { editor, store } = createHarness()
    editor.openCreate({ kind: 'providerEndpoint', id: 7 })
    editor.form.value.name = 'Fresh Key'
    editor.form.value.modelValues = ['gpt-4o']

    await editor.save()
    expect(editor.errors.value.secret).toBe('新增账号必须输入 API 密钥')
    expect(store.saveAccount).not.toHaveBeenCalled()
  })

  it('saves through the store and closes on success', async () => {
    const { editor, store } = createHarness()
    editor.openCreate({ kind: 'providerEndpoint', id: 7 })
    editor.form.value.name = 'Fresh Key'
    editor.form.value.secret = 'sk-test'
    editor.form.value.modelValues = ['gpt-4o']

    await editor.save()

    expect(store.saveAccount).toHaveBeenCalledTimes(1)
    expect(store.saveAccount).toHaveBeenCalledWith(
      expect.objectContaining({ isEditing: false, accountId: 0 }),
    )
    expect(editor.visible.value).toBe(false)
  })
})
