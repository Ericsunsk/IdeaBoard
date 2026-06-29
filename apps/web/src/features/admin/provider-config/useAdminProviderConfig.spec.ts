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
    const message = createMessagePort()
    mockApi.fetchAdminAccountLimits.mockRejectedValueOnce(new Error('limits failed'))

    const context = useAdminProviderConfig({
      message,
      confirm: { danger: vi.fn() },
    })

    await context.commands.loadData()

    expect(context.accountEditor.accountLimits.value).toEqual(FALLBACK_ADMIN_ACCOUNT_LIMITS)
    expect(message.warning).toHaveBeenCalledWith('账号限制加载失败，已使用默认限制')
    expect(warnSpy).toHaveBeenCalledWith(
      '[provider-config] failed to load account limits, using fallback',
      expect.any(Error),
    )
  })
})

function createMessagePort() {
  return {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
  }
}
