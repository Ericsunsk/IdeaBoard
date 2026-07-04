import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminProviderConfig } from './useAdminProviderConfig'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigApi'

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
  mockApi.fetchAdminChannels.mockResolvedValue([])
  mockApi.fetchAllAdminAccounts.mockResolvedValue([])
  mockApi.fetchAdminAccountLimits.mockResolvedValue(FALLBACK_ADMIN_ACCOUNT_LIMITS)
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
      confirm: { danger: vi.fn() },
    })

    await context.commands.loadData()

    expect(context.accountEditor.accountLimits.value).toEqual(FALLBACK_ADMIN_ACCOUNT_LIMITS)
    expect(feedback.accountLimitsFallback).toHaveBeenCalledWith(error)
    expect(warnSpy).toHaveBeenCalledWith(
      '[provider-config] failed to load account limits, using fallback',
      expect.any(Error),
    )
  })
})

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
