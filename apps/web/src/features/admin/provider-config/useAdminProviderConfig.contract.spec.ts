import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminProviderConfig } from './useAdminProviderConfig'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigApi'

const mockApi = vi.hoisted(() => ({
  createAdminAccountRecord: vi.fn(),
  deleteAdminAccountRecord: vi.fn(),
  deleteAdminProviderEndpoint: vi.fn(),
  detectAdminProviderEndpointModels: vi.fn(),
  fetchAdminAccountLimits: vi.fn(),
  fetchAllAdminAccounts: vi.fn(),
  fetchAdminProviderEndpoints: vi.fn(),
  patchAdminAccountModelRecord: vi.fn(),
  saveAdminProviderEndpoint: vi.fn(),
  updateAdminAccountRecord: vi.fn(),
}))

vi.mock('./providerConfigApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./providerConfigApi')>()),
  createAdminAccountRecord: mockApi.createAdminAccountRecord,
  deleteAdminAccountRecord: mockApi.deleteAdminAccountRecord,
  deleteAdminProviderEndpoint: mockApi.deleteAdminProviderEndpoint,
  detectAdminProviderEndpointModels: mockApi.detectAdminProviderEndpointModels,
  fetchAdminAccountLimits: mockApi.fetchAdminAccountLimits,
  fetchAllAdminAccounts: mockApi.fetchAllAdminAccounts,
  fetchAdminProviderEndpoints: mockApi.fetchAdminProviderEndpoints,
  patchAdminAccountModelRecord: mockApi.patchAdminAccountModelRecord,
  saveAdminProviderEndpoint: mockApi.saveAdminProviderEndpoint,
  updateAdminAccountRecord: mockApi.updateAdminAccountRecord,
}))

beforeEach(() => {
  vi.clearAllMocks()
  mockApi.fetchAdminProviderEndpoints.mockResolvedValue([])
  mockApi.fetchAllAdminAccounts.mockResolvedValue([])
  mockApi.fetchAdminAccountLimits.mockResolvedValue(FALLBACK_ADMIN_ACCOUNT_LIMITS)
  mockApi.patchAdminAccountModelRecord.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useAdminProviderConfig', () => {
  it('warns and exposes fallback account limits when limits loading fails', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const feedback = createProviderConfigFeedback()
    const error = new Error('limits failed')
    mockApi.fetchAdminAccountLimits.mockRejectedValueOnce(error)

    const context = useAdminProviderConfig({
      feedback,
      confirm: { request: vi.fn() },
    })

    await context.commands.loadData()

    expect(context.accountEditor.accountLimits.value).toEqual(FALLBACK_ADMIN_ACCOUNT_LIMITS)
    expect(feedback.accountLimitsFallback).toHaveBeenCalledWith(error)
    expect(warnSpy).toHaveBeenCalledWith(
      '[provider-config] failed to load account limits, using fallback',
      expect.any(Error),
    )
  })

  it('fills empty-query model suggestions when opening the model selector detects models', async () => {
    mockApi.fetchAdminProviderEndpoints.mockResolvedValueOnce([
      {
        id: 1,
        apiType: 'openai-compatible',
        name: 'OpenAI ProviderEndpoint',
        baseUrl: 'https://api.example.com/v1',
        status: 1,
        createdAt: '',
        updatedAt: '',
      },
    ])
    mockApi.detectAdminProviderEndpointModels.mockResolvedValueOnce(['gpt-4o', 'imagen-4'])
    const context = useAdminProviderConfig({
      feedback: createProviderConfigFeedback(),
      confirm: { request: vi.fn() },
    })

    await context.commands.loadData()
    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 1 })
    context.accountEditor.form.value.secret = 'sk-live'

    await context.accountEditor.handleModelSelectShow(true)

    expect(mockApi.detectAdminProviderEndpointModels).toHaveBeenCalledWith({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-live',
      apiType: 'openai-compatible',
    })
    expect(context.accountEditor.modelSuggestions.value).toEqual(['gpt-4o', 'imagen-4'])
  })

  it('creates a uniquely named APIKey account with multiple assigned models per providerEndpoint', async () => {
    mockApi.fetchAdminProviderEndpoints.mockResolvedValueOnce([
      {
        id: 1,
        apiType: 'openai-compatible',
        name: 'OpenAI ProviderEndpoint',
        baseUrl: 'https://api.example.com/v1',
        status: 1,
        createdAt: '',
        updatedAt: '',
      },
    ])
    mockApi.fetchAllAdminAccounts.mockResolvedValueOnce([
      {
        id: 11,
        name: 'OpenAI ProviderEndpoint APIKey',
        providerEndpointId: 1,
        apiType: 'openai-compatible',
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
    mockApi.createAdminAccountRecord.mockResolvedValueOnce(undefined)
    const context = useAdminProviderConfig({
      feedback: createProviderConfigFeedback(),
      confirm: { request: vi.fn() },
    })

    await context.commands.loadData()
    context.accountEditor.openCreate({ kind: 'providerEndpoint', id: 1 })

    expect(context.accountEditor.form.value.name).toBe('OpenAI ProviderEndpoint APIKey 2')

    context.accountEditor.form.value.secret = 'sk-live'
    context.accountEditor.form.value.modelValues = ['gpt-4o', 'imagen-4']
    await context.accountEditor.save()

    expect(mockApi.createAdminAccountRecord).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'OpenAI ProviderEndpoint APIKey 2',
        models: [
          expect.objectContaining({ name: 'gpt-4o' }),
          expect.objectContaining({ name: 'imagen-4' }),
        ],
      }),
    )
  })
})

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
