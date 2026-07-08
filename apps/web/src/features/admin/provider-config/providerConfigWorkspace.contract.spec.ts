import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigLimits'
import { createAdminProviderConfigWorkspace } from './providerConfigWorkspace'
import type { AdminProviderConfigResource } from './providerConfigResource'
import type {
  AdminProviderAccountLimits,
  ProviderAccountRecord,
  ProviderEndpointRecord,
} from './providerConfigTypes'

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
  mockResource.fetchProviderEndpoints.mockResolvedValue([makeProviderEndpoint()])
  mockResource.fetchAccounts.mockResolvedValue([
    makeAccount({
      models: [makeModel({ name: 'gpt-4o', enabled: true, weight: 7 })],
    }),
  ])
  mockResource.fetchAccountLimits.mockResolvedValue(makeAccountLimits())
  mockResource.createAccountRecord.mockResolvedValue(undefined)
  mockResource.deleteAccountRecord.mockResolvedValue(undefined)
  mockResource.deleteProviderEndpoint.mockResolvedValue(undefined)
  mockResource.detectProviderEndpointModels.mockResolvedValue([])
  mockResource.patchAccountModelRecord.mockResolvedValue(
    makeAccount({
      models: [makeModel({ name: 'gpt-4o', enabled: false, weight: 7 })],
      updatedAt: 'patched-at',
    }),
  )
  mockResource.saveProviderEndpoint.mockResolvedValue(undefined)
  mockResource.updateAccountRecord.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('provider config workspace facade', () => {
  it('loads providerEndpoints, accounts, account limits, and card view models', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadData()

    expect(mockResource.fetchProviderEndpoints).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchAccounts).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchAccountLimits).toHaveBeenCalledTimes(1)
    expect(workspace.accountLimits.value).toEqual(makeAccountLimits())
    expect(workspace.cards.items.value).toHaveLength(1)
    expect(workspace.cards.items.value[0]?.providerEndpoint.name).toBe('OpenAI ProviderEndpoint')
    expect(workspace.cards.items.value[0]?.accounts[0]?.concurrencyLabel).toBe('1/3')
    expect(workspace.cards.totalProviderEndpointsLabel.value).toBe('1 端点')
    expect(workspace.cards.accountRunningLabel.value).toBe('1/1 运行中')
    expect(workspace.cards.activeModelCount.value).toBe(1)

    workspace.cards.searchKeyword.value = 'missing'

    expect(workspace.cards.items.value).toHaveLength(0)
    expect(workspace.cards.totalProviderEndpointsLabel.value).toBe('0 匹配端点')
    expect(workspace.cards.emptyMessage.value).toBe('无匹配供应商端点')
  })

  it('saves a providerEndpoint then reloads through the workspace command sequence', async () => {
    const { workspace, feedback } = createWorkspace()
    const afterSaved = vi.fn()
    const data = {
      apiType: 'openai-compatible' as const,
      name: 'Updated ProviderEndpoint',
      baseUrl: 'https://api.updated.example/v1',
      status: 1,
    }

    await workspace.loadData()
    await workspace.saveProviderEndpoint({
      id: 1,
      data,
      isEditing: true,
      afterSaved,
    })

    expect(mockResource.saveProviderEndpoint).toHaveBeenCalledWith({ id: 1, data })
    expect(feedback.providerEndpointSaved).toHaveBeenCalledWith(true)
    expect(afterSaved).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchProviderEndpoints).toHaveBeenCalledTimes(2)
    expect(mockResource.fetchAccounts).toHaveBeenCalledTimes(2)
    expect(mockResource.fetchAccountLimits).toHaveBeenCalledTimes(2)
  })

  it('maps providerEndpoint save failures without closing or refreshing the workspace', async () => {
    const { workspace, feedback } = createWorkspace()
    const afterSaved = vi.fn()
    const error = new Error('save failed')
    mockResource.saveProviderEndpoint.mockRejectedValueOnce(error)

    await workspace.loadData()
    await workspace.saveProviderEndpoint({
      data: {
        apiType: 'openai-compatible',
        name: 'Broken ProviderEndpoint',
        baseUrl: 'https://api.broken.example/v1',
        status: 1,
      },
      isEditing: false,
      afterSaved,
    })

    expect(feedback.providerEndpointSaveFailed).toHaveBeenCalledWith(error)
    expect(feedback.providerEndpointSaved).not.toHaveBeenCalled()
    expect(afterSaved).not.toHaveBeenCalled()
    expect(mockResource.fetchProviderEndpoints).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchAccounts).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchAccountLimits).toHaveBeenCalledTimes(1)
  })

  it('saves an account while preserving existing model settings', async () => {
    const { workspace, feedback } = createWorkspace()
    const afterSaved = vi.fn()
    const selectedProviderEndpoint = makeProviderEndpoint()

    await workspace.loadData()
    await workspace.saveAccount({
      isEditing: true,
      accountId: 11,
      form: {
        providerEndpointId: 1,
        name: 'OpenAI Account',
        modelValues: ['gpt-4o', 'imagen-4'],
        secret: 'sk-new',
        maxConcurrent: 5,
        timeoutSeconds: 180,
      },
      selectedProviderEndpoint,
      modelValues: ['gpt-4o', 'imagen-4'],
      secret: 'sk-new',
      afterSaved,
    })

    expect(mockResource.updateAccountRecord).toHaveBeenCalledWith(
      11,
      expect.objectContaining({
        name: 'OpenAI Account',
        max_concurrent: 5,
        timeout: 180,
        secret: 'sk-new',
        models: [
          expect.objectContaining({ name: 'gpt-4o', enabled: true, weight: 7 }),
          expect.objectContaining({ name: 'imagen-4', enabled: true, weight: 1 }),
        ],
      }),
    )
    expect(feedback.accountSaved).toHaveBeenCalledWith(true)
    expect(afterSaved).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchProviderEndpoints).toHaveBeenCalledTimes(2)
    expect(mockResource.fetchAccounts).toHaveBeenCalledTimes(2)
    expect(mockResource.fetchAccountLimits).toHaveBeenCalledTimes(2)
  })

  it('detects models with missing-key feedback and per-session caching', async () => {
    const { workspace, feedback } = createWorkspace()
    const providerEndpoint = makeProviderEndpoint()
    mockResource.detectProviderEndpointModels.mockResolvedValueOnce([
      'gpt-4o',
      ' GPT-4o ',
      'imagen-4',
    ])

    const missingKeyResult = await workspace.detectModels({
      providerEndpoint,
      secret: '',
      detectedValues: [],
      sessionCache: new Set(),
      hasShownApiKeyHint: false,
    })

    expect(feedback.missingApiKeyForModelDetection).toHaveBeenCalledTimes(1)
    expect(missingKeyResult.hasShownApiKeyHint).toBe(true)
    expect(mockResource.detectProviderEndpointModels).not.toHaveBeenCalled()

    const detectedResult = await workspace.detectModels({
      providerEndpoint,
      secret: ' sk-live ',
      detectedValues: ['existing-model'],
      sessionCache: new Set(),
      hasShownApiKeyHint: false,
    })

    expect(mockResource.detectProviderEndpointModels).toHaveBeenCalledWith({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-live',
      apiType: 'openai-compatible',
    })
    expect(detectedResult.detectedValues).toEqual(['existing-model', 'gpt-4o', 'imagen-4'])

    await workspace.detectModels({
      providerEndpoint,
      secret: 'sk-live',
      detectedValues: detectedResult.detectedValues,
      sessionCache: detectedResult.sessionCache,
      hasShownApiKeyHint: false,
    })

    expect(mockResource.detectProviderEndpointModels).toHaveBeenCalledTimes(1)
  })

  it('does not cache failed model detection attempts', async () => {
    const { workspace, feedback } = createWorkspace()
    const providerEndpoint = makeProviderEndpoint()
    const error = new Error('detect failed')
    mockResource.detectProviderEndpointModels
      .mockRejectedValueOnce(error)
      .mockResolvedValueOnce(['gpt-4o'])

    const failedResult = await workspace.detectModels({
      providerEndpoint,
      secret: 'sk-live',
      detectedValues: [],
      sessionCache: new Set(),
      hasShownApiKeyHint: false,
    })

    expect(feedback.modelDetectionFailed).toHaveBeenCalledWith(error)
    expect(failedResult.sessionCache.size).toBe(0)

    const retryResult = await workspace.detectModels({
      providerEndpoint,
      secret: 'sk-live',
      detectedValues: failedResult.detectedValues,
      sessionCache: failedResult.sessionCache,
      hasShownApiKeyHint: false,
    })

    expect(mockResource.detectProviderEndpointModels).toHaveBeenCalledTimes(2)
    expect(retryResult.detectedValues).toEqual(['gpt-4o'])
    expect(retryResult.sessionCache.size).toBe(1)
  })

  it('toggles account model status and updates local card rows', async () => {
    const { workspace, feedback } = createWorkspace()

    await workspace.loadData()
    await workspace.toggleAccountModelStatus({ kind: 'account', id: 11 }, 'GPT-4O')

    expect(mockResource.patchAccountModelRecord).toHaveBeenCalledWith(11, {
      name: 'gpt-4o',
      enabled: false,
    })
    expect(mockResource.updateAccountRecord).not.toHaveBeenCalled()
    expect(feedback.modelStatusChanged).toHaveBeenCalledWith(false)
    expect(workspace.getAccountModelRows(workspace.cards.items.value[0]!.accounts[0]!)).toEqual([
      { name: 'gpt-4o', disabled: true },
    ])
  })

  it('keeps stale target feedback and delete confirmation in the facade', async () => {
    const { workspace, feedback, confirm } = createWorkspace()

    workspace.deleteProviderEndpoint({ kind: 'providerEndpoint', id: 404 })

    expect(feedback.staleProviderEndpoint).toHaveBeenCalledTimes(1)
    expect(confirm.request).not.toHaveBeenCalled()

    await workspace.loadData()
    workspace.deleteProviderEndpoint({ kind: 'providerEndpoint', id: 1 })

    expect(confirm.request).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认删除',
        message: '确定删除供应商端点「OpenAI ProviderEndpoint」吗？此操作不可恢复。',
        confirmLabel: '确定',
        intent: 'danger',
      }),
    )

    await confirm.request.mock.calls[0]?.[0].onConfirm()

    expect(mockResource.deleteProviderEndpoint).toHaveBeenCalledWith(1)
    expect(feedback.providerEndpointDeleted).toHaveBeenCalledTimes(1)
  })

  it('falls back to default account limits when limit loading fails', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { workspace, feedback } = createWorkspace()
    const error = new Error('limits failed')
    mockResource.fetchAccountLimits.mockRejectedValueOnce(error)

    await workspace.loadData()

    expect(workspace.accountLimits.value).toEqual(FALLBACK_ADMIN_ACCOUNT_LIMITS)
    expect(feedback.accountLimitsFallback).toHaveBeenCalledWith(error)
    expect(warnSpy).toHaveBeenCalledWith(
      '[provider-config] failed to load account limits, using fallback',
      expect.any(Error),
    )
  })
})

function createWorkspace() {
  const feedback = createProviderConfigFeedback()
  const confirm = { request: vi.fn() }
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminProviderConfigWorkspace({
    feedback,
    confirm,
    runAsyncAction,
    resource: mockResource as unknown as AdminProviderConfigResource,
  })
  return { workspace, feedback, confirm }
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

function makeAccountLimits(): AdminProviderAccountLimits {
  return {
    maxConcurrent: { min: 1, max: 10, default: 4 },
    timeoutSeconds: { min: 10, max: 300, default: 120 },
  }
}

function makeProviderEndpoint(
  overrides: Partial<ProviderEndpointRecord> = {},
): ProviderEndpointRecord {
  return {
    id: 1,
    apiType: 'openai-compatible',
    name: 'OpenAI ProviderEndpoint',
    baseUrl: 'https://api.example.com/v1',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function makeAccount(overrides: Partial<ProviderAccountRecord> = {}): ProviderAccountRecord {
  return {
    id: 11,
    name: 'OpenAI Account',
    providerEndpointId: 1,
    apiType: 'openai-compatible',
    baseUrl: 'https://api.example.com/v1',
    models: [],
    secretConfigured: true,
    maxConcurrent: 3,
    currentConcurrent: 1,
    waitQueueCount: 0,
    runningStatus: '',
    failCount: 0,
    weight: 100,
    timeoutSeconds: 120,
    extraConfig: '{}',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function makeModel(overrides: Partial<ProviderAccountRecord['models'][number]> = {}) {
  return {
    name: 'model',
    enabled: true,
    weight: 1,
    maxConcurrent: undefined,
    featureSlugs: undefined,
    sortOrder: 0,
    ...overrides,
  }
}
