import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigApi'
import { createAdminProviderConfigWorkspace } from './providerConfigWorkspace'
import type {
  AdminProviderAccountLimits,
  ProviderAccountRecord,
  ProviderChannelRecord,
} from './providerConfigTypes'

const mockApi = vi.hoisted(() => ({
  createAdminAccountRecord: vi.fn(),
  deleteAdminAccountRecord: vi.fn(),
  deleteAdminChannel: vi.fn(),
  detectAdminChannelModels: vi.fn(),
  fetchAdminAccountLimits: vi.fn(),
  fetchAllAdminAccounts: vi.fn(),
  fetchAdminChannels: vi.fn(),
  saveAdminChannel: vi.fn(),
  updateAdminAccountRecord: vi.fn(),
}))

vi.mock('./providerConfigApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./providerConfigApi')>()),
  createAdminAccountRecord: mockApi.createAdminAccountRecord,
  deleteAdminAccountRecord: mockApi.deleteAdminAccountRecord,
  deleteAdminChannel: mockApi.deleteAdminChannel,
  detectAdminChannelModels: mockApi.detectAdminChannelModels,
  fetchAdminAccountLimits: mockApi.fetchAdminAccountLimits,
  fetchAllAdminAccounts: mockApi.fetchAllAdminAccounts,
  fetchAdminChannels: mockApi.fetchAdminChannels,
  saveAdminChannel: mockApi.saveAdminChannel,
  updateAdminAccountRecord: mockApi.updateAdminAccountRecord,
}))

beforeEach(() => {
  vi.clearAllMocks()
  mockApi.fetchAdminChannels.mockResolvedValue([makeChannel()])
  mockApi.fetchAllAdminAccounts.mockResolvedValue([
    makeAccount({
      models: [makeModel({ name: 'gpt-4o', enabled: true, weight: 7 })],
    }),
  ])
  mockApi.fetchAdminAccountLimits.mockResolvedValue(makeAccountLimits())
  mockApi.createAdminAccountRecord.mockResolvedValue(undefined)
  mockApi.deleteAdminAccountRecord.mockResolvedValue(undefined)
  mockApi.deleteAdminChannel.mockResolvedValue(undefined)
  mockApi.detectAdminChannelModels.mockResolvedValue([])
  mockApi.saveAdminChannel.mockResolvedValue(undefined)
  mockApi.updateAdminAccountRecord.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('provider config workspace facade', () => {
  it('loads channels, accounts, account limits, and card view models', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadData()

    expect(mockApi.fetchAdminChannels).toHaveBeenCalledTimes(1)
    expect(mockApi.fetchAllAdminAccounts).toHaveBeenCalledTimes(1)
    expect(mockApi.fetchAdminAccountLimits).toHaveBeenCalledTimes(1)
    expect(workspace.accountLimits.value).toEqual(makeAccountLimits())
    expect(workspace.cards.items.value).toHaveLength(1)
    expect(workspace.cards.items.value[0]?.channel.name).toBe('OpenAI Channel')
    expect(workspace.cards.items.value[0]?.accounts[0]?.concurrencyLabel).toBe('1/3')
    expect(workspace.cards.totalChannelsLabel.value).toBe('1 渠道')
    expect(workspace.cards.accountRunningLabel.value).toBe('1/1 运行中')
    expect(workspace.cards.activeModelCount.value).toBe(1)

    workspace.cards.searchKeyword.value = 'missing'

    expect(workspace.cards.items.value).toHaveLength(0)
    expect(workspace.cards.totalChannelsLabel.value).toBe('0 匹配渠道')
    expect(workspace.cards.emptyMessage.value).toBe('无匹配渠道商')
  })

  it('saves a channel then reloads through the workspace command sequence', async () => {
    const { workspace, feedback } = createWorkspace()
    const afterSaved = vi.fn()
    const data = {
      apiType: 'openai-compatible' as const,
      name: 'Updated Channel',
      baseUrl: 'https://api.updated.example/v1',
    }

    await workspace.loadData()
    await workspace.saveChannel({
      id: 1,
      data,
      isEditing: true,
      afterSaved,
    })

    expect(mockApi.saveAdminChannel).toHaveBeenCalledWith({ id: 1, data })
    expect(feedback.channelSaved).toHaveBeenCalledWith(true)
    expect(afterSaved).toHaveBeenCalledTimes(1)
    expect(mockApi.fetchAdminChannels).toHaveBeenCalledTimes(2)
  })

  it('saves an account while preserving existing model settings', async () => {
    const { workspace, feedback } = createWorkspace()
    const afterSaved = vi.fn()
    const selectedChannel = makeChannel()

    await workspace.loadData()
    await workspace.saveAccount({
      isEditing: true,
      accountId: 11,
      form: {
        channelId: 1,
        modelValues: ['gpt-4o', 'imagen-4'],
        secret: 'sk-new',
        maxConcurrent: 5,
        timeoutSeconds: 180,
      },
      selectedChannel,
      modelValues: ['gpt-4o', 'imagen-4'],
      secret: 'sk-new',
      afterSaved,
    })

    expect(mockApi.updateAdminAccountRecord).toHaveBeenCalledWith(
      11,
      expect.objectContaining({
        name: 'OpenAI Channel',
        platform: 'openai-compatible',
        api_type: 'openai-compatible',
        proxy_url: 'https://api.example.com/v1',
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
    expect(mockApi.fetchAdminChannels).toHaveBeenCalledTimes(2)
  })

  it('detects models with missing-key feedback and per-session caching', async () => {
    const { workspace, feedback } = createWorkspace()
    const channel = makeChannel()
    mockApi.detectAdminChannelModels.mockResolvedValueOnce(['gpt-4o', ' GPT-4o ', 'imagen-4'])

    const missingKeyResult = await workspace.detectModels({
      channel,
      secret: '',
      detectedValues: [],
      sessionCache: new Set(),
      hasShownApiKeyHint: false,
    })

    expect(feedback.missingApiKeyForModelDetection).toHaveBeenCalledTimes(1)
    expect(missingKeyResult.hasShownApiKeyHint).toBe(true)
    expect(mockApi.detectAdminChannelModels).not.toHaveBeenCalled()

    const detectedResult = await workspace.detectModels({
      channel,
      secret: ' sk-live ',
      detectedValues: ['existing-model'],
      sessionCache: new Set(),
      hasShownApiKeyHint: false,
    })

    expect(mockApi.detectAdminChannelModels).toHaveBeenCalledWith({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-live',
      apiType: 'openai-compatible',
    })
    expect(detectedResult.detectedValues).toEqual([
      'existing-model',
      'gpt-4o',
      'imagen-4',
    ])

    await workspace.detectModels({
      channel,
      secret: 'sk-live',
      detectedValues: detectedResult.detectedValues,
      sessionCache: detectedResult.sessionCache,
      hasShownApiKeyHint: false,
    })

    expect(mockApi.detectAdminChannelModels).toHaveBeenCalledTimes(1)
  })

  it('toggles account model status and updates local card rows', async () => {
    const { workspace, feedback } = createWorkspace()

    await workspace.loadData()
    await workspace.toggleAccountModelStatus({ kind: 'account', id: 11 }, 'GPT-4O')

    expect(mockApi.updateAdminAccountRecord).toHaveBeenCalledWith(
      11,
      expect.objectContaining({
        models: [expect.objectContaining({ name: 'gpt-4o', enabled: false, weight: 7 })],
      }),
    )
    expect(feedback.modelStatusChanged).toHaveBeenCalledWith(false)
    expect(
      workspace.getAccountModelRows(workspace.cards.items.value[0]!.accounts[0]!),
    ).toEqual([{ name: 'gpt-4o', disabled: true }])
  })

  it('keeps stale target feedback and delete confirmation in the facade', async () => {
    const { workspace, feedback, confirm } = createWorkspace()

    workspace.deleteChannel({ kind: 'channel', id: 404 })

    expect(feedback.staleChannel).toHaveBeenCalledTimes(1)
    expect(confirm.danger).not.toHaveBeenCalled()

    await workspace.loadData()
    workspace.deleteChannel({ kind: 'channel', id: 1 })

    expect(confirm.danger).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认删除',
        content: '确定删除渠道商「OpenAI Channel」吗？此操作不可恢复。',
        acceptLabel: '确定',
      }),
    )

    await confirm.danger.mock.calls[0]?.[0].onConfirm()

    expect(mockApi.deleteAdminChannel).toHaveBeenCalledWith(1)
    expect(feedback.channelDeleted).toHaveBeenCalledTimes(1)
  })

  it('falls back to default account limits when limit loading fails', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { workspace, feedback } = createWorkspace()
    const error = new Error('limits failed')
    mockApi.fetchAdminAccountLimits.mockRejectedValueOnce(error)

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
  const confirm = { danger: vi.fn() }
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminProviderConfigWorkspace({
    feedback,
    confirm,
    runAsyncAction,
  })
  return { workspace, feedback, confirm }
}

function createProviderConfigFeedback() {
  return {
    accountLimitsFallback: vi.fn(),
    loadFailed: vi.fn(),
    staleChannel: vi.fn(),
    staleAccount: vi.fn(),
    staleModel: vi.fn(),
    channelFormInvalid: vi.fn(),
    accountFormInvalid: vi.fn(),
    channelSaved: vi.fn(),
    channelSaveFailed: vi.fn(),
    channelDeleted: vi.fn(),
    channelDeleteFailed: vi.fn(),
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

function makeChannel(overrides: Partial<ProviderChannelRecord> = {}): ProviderChannelRecord {
  return {
    id: 1,
    apiType: 'openai-compatible',
    name: 'OpenAI Channel',
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
    channelId: 1,
    platform: 'openai-compatible',
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
