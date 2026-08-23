import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminProviderConfig } from './useAdminProviderConfig'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigLimits'
import type { AdminProviderConfigResource } from './providerConfigResource'

const mockResource = {
  createAccountRecord: vi.fn(),
  deleteAccountRecord: vi.fn(),
  deleteProviderEndpoint: vi.fn(),
  detectProviderEndpointModels: vi.fn(),
  fetchAccountLimits: vi.fn(),
  fetchAccounts: vi.fn(),
  fetchProviderEndpoints: vi.fn(),
  patchAccountModelRecord: vi.fn(),
  saveProviderEndpoint: vi.fn(),
  updateAccountRecord: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  mockResource.fetchProviderEndpoints.mockResolvedValue([])
  mockResource.fetchAccounts.mockResolvedValue([])
  mockResource.fetchAccountLimits.mockResolvedValue(FALLBACK_ADMIN_ACCOUNT_LIMITS)
  mockResource.patchAccountModelRecord.mockResolvedValue(undefined)
  mockResource.createAccountRecord.mockResolvedValue(undefined)
  mockResource.updateAccountRecord.mockResolvedValue(undefined)
  mockResource.saveProviderEndpoint.mockResolvedValue(undefined)
  mockResource.deleteProviderEndpoint.mockResolvedValue(undefined)
  mockResource.deleteAccountRecord.mockResolvedValue(undefined)
  mockResource.detectProviderEndpointModels.mockResolvedValue([])
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useAdminProviderConfig', () => {
  it('warns and exposes fallback account limits when limits loading fails', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const feedback = createProviderConfigFeedback()
    const error = new Error('limits failed')
    mockResource.fetchAccountLimits.mockRejectedValueOnce(error)

    const context = createContext(feedback)

    await context.commands.loadData()

    expect(context.accountEditor.accountLimits.value).toEqual(FALLBACK_ADMIN_ACCOUNT_LIMITS)
    expect(feedback.accountLimitsFallback).toHaveBeenCalledWith(error)
    expect(warnSpy).toHaveBeenCalledWith(
      '[provider-config] failed to load account limits, using fallback',
      expect.any(Error),
    )
  })

  it('fills empty-query model suggestions when opening the model selector detects models', async () => {
    mockResource.fetchProviderEndpoints.mockResolvedValueOnce([
      {
        id: 1,
        apiType: 'openai-compatible',
        protocol: 'model-api',
        capabilities: ['image-generation'],
        name: 'OpenAI ProviderEndpoint',
        baseUrl: 'https://api.example.com/v1',
        apiKeyConfigured: false,
        status: 1,
        createdAt: '',
        updatedAt: '',
      },
    ])
    mockResource.detectProviderEndpointModels.mockResolvedValueOnce(['gpt-4o', 'imagen-4'])
    const context = createContext()

    await context.commands.loadData()
    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 1 })
    context.accountEditor.form.value.secret = 'sk-live'

    await context.accountEditor.handleModelSelectShow(true)

    expect(mockResource.detectProviderEndpointModels).toHaveBeenCalledWith({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-live',
      apiType: 'openai-compatible',
    })
    expect(context.accountEditor.modelSuggestions.value).toEqual(['gpt-4o', 'imagen-4'])
  })

  it('resets the model search input when the account editor closes or changes endpoint', async () => {
    mockResource.fetchProviderEndpoints.mockResolvedValueOnce([
      makeProviderEndpoint({ id: 1, name: 'First endpoint' }),
      makeProviderEndpoint({ id: 2, name: 'Second endpoint' }),
    ])
    const context = createContext()

    await context.commands.loadData()
    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 1 })
    context.accountEditor.modelSearchInput.value = 'gpt-4o'

    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 2 })

    expect(context.accountEditor.modelSearchInput.value).toBe('')

    context.accountEditor.modelSearchInput.value = 'imagen-4'
    context.accountEditor.close()

    expect(context.accountEditor.visible.value).toBe(false)
    expect(context.accountEditor.modelSearchInput.value).toBe('')
  })

  it('creates a uniquely named APIKey account with multiple assigned models per providerEndpoint', async () => {
    mockResource.fetchProviderEndpoints.mockResolvedValueOnce([
      {
        id: 1,
        apiType: 'openai-compatible',
        protocol: 'model-api',
        capabilities: ['image-generation'],
        name: 'OpenAI ProviderEndpoint',
        baseUrl: 'https://api.example.com/v1',
        apiKeyConfigured: false,
        status: 1,
        createdAt: '',
        updatedAt: '',
      },
    ])
    mockResource.fetchAccounts.mockResolvedValueOnce([
      {
        id: 11,
        name: 'OpenAI ProviderEndpoint APIKey',
        providerEndpointId: 1,
        apiType: 'openai-compatible',
        protocol: 'model-api',
        capabilities: ['image-generation'],
        baseUrl: 'https://api.example.com/v1',
        models: [{ name: 'gpt-4o', enabled: true, weight: 1, sortOrder: 0 }],
        secretConfigured: true,
        maxConcurrent: 3,
        currentConcurrent: 0,
        waitQueueCount: 0,
        runningStatus: '',
        failCount: 0,
        weight: 100,
        timeoutSeconds: 120,
        extraConfig: '{}',
        status: 1,
        createdAt: '',
        updatedAt: '',
      },
    ])
    mockResource.createAccountRecord.mockResolvedValueOnce(undefined)
    const context = createContext()

    await context.commands.loadData()
    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 1 })

    expect(context.accountEditor.form.value.name).toBe('OpenAI ProviderEndpoint APIKey 2')

    context.accountEditor.form.value.secret = 'sk-live'
    context.accountEditor.form.value.modelValues = ['gpt-4o', 'imagen-4']
    await context.accountEditor.save()

    expect(mockResource.createAccountRecord).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'OpenAI ProviderEndpoint APIKey 2',
        models: [
          expect.objectContaining({ name: 'gpt-4o' }),
          expect.objectContaining({ name: 'imagen-4' }),
        ],
      }),
    )
  })

  it('rejects an image processing endpoint without a selected capability', async () => {
    const feedback = createProviderConfigFeedback()
    const context = createContext(feedback)

    context.providerEndpointEditor.openCreate()
    context.providerEndpointEditor.form.value = {
      apiType: 'openai-compatible',
      protocol: 'image-url-json-to-image',
      capability: '',
      name: 'Modal Image Processing',
      baseUrl: 'https://example.com/image-processing',
      apiKey: 'modal-secret',
      status: 1,
    }

    await context.providerEndpointEditor.save()

    expect(feedback.providerEndpointFormInvalid).toHaveBeenCalledOnce()
    expect(context.providerEndpointEditor.errors.value.capability).toBe('请选择处理功能')
    expect(mockResource.saveProviderEndpoint).not.toHaveBeenCalled()
  })
})

function createContext(feedback = createProviderConfigFeedback()) {
  return useAdminProviderConfig({
    feedback,
    confirm: { request: vi.fn() },
    resource: mockResource as unknown as AdminProviderConfigResource,
  })
}

function createProviderConfigFeedback() {
  return {
    accountLimitsFallback: vi.fn(),
    loadFailed: vi.fn(),
    staleProviderEndpoint: vi.fn(),
    staleAccount: vi.fn(),
    staleModel: vi.fn(),
    providerEndpointFormInvalid: vi.fn(),
    accountFormInvalid: vi.fn(),
    providerEndpointSaved: vi.fn(),
    providerEndpointSaveFailed: vi.fn(),
    providerEndpointDeleted: vi.fn(),
    providerEndpointDeleteFailed: vi.fn(),
    accountSaved: vi.fn(),
    accountSaveFailed: vi.fn(),
    accountDeleted: vi.fn(),
    accountDeleteFailed: vi.fn(),
    modelStatusChanged: vi.fn(),
    modelStatusChangeFailed: vi.fn(),
    missingApiKeyForModelDetection: vi.fn(),
    modelDetectionFailed: vi.fn(),
  }
}

function makeProviderEndpoint(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    apiType: 'openai-compatible',
    protocol: 'model-api',
    capabilities: ['image-generation'],
    name: 'OpenAI ProviderEndpoint',
    baseUrl: 'https://api.example.com/v1',
    apiKeyConfigured: false,
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}
