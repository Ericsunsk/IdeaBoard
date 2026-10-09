import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import AdminOAuthLoginCard from './AdminOAuthLoginCard.vue'
import type { OAuthProviderSettingsResource } from './resource'

vi.mock('../../../../shared/composables/useAppToast')
vi.mock('../../shared/logos/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

function renderCard(resource?: OAuthProviderSettingsResource) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(AdminOAuthLoginCard, {
    props: { resource },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

const summaries = {
  google: {
    enabled: true,
    configured: true,
    clientId: 'google-client.apps.googleusercontent.com',
    clientSecretConfigured: true,
    callbackUrl: 'https://example.com/api/auth/callback/google',
  },
  discord: {
    enabled: false,
    configured: false,
    clientId: '',
    clientSecretConfigured: false,
    callbackUrl: 'https://example.com/api/auth/callback/discord',
  },
}

describe('AdminOAuthLoginCard', () => {
  it('renders provider statuses and opens the settings dialog from configure', async () => {
    mockAppToast()
    const resource = {
      get: vi.fn(async (provider: 'google' | 'discord') => summaries[provider]),
      save: vi.fn(),
    }
    const { user } = renderCard(resource as unknown as OAuthProviderSettingsResource)

    expect(await screen.findByText('OAuth')).toBeVisible()
    expect(await screen.findByText('Google')).toBeVisible()
    expect(await screen.findByText('Discord')).toBeVisible()
    expect(resource.get).toHaveBeenCalledWith('google')
    expect(resource.get).toHaveBeenCalledWith('discord')

    await user.click(screen.getByRole('button', { name: '配置' }))
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    expect(within(dialog).getByText('Google 凭据配置')).toBeVisible()
  })

  it('renders a retryable load error', async () => {
    mockAppToast()
    const resource = {
      get: vi.fn().mockRejectedValue(new Error('oauth down')),
      save: vi.fn(),
    }
    const { user } = renderCard(resource as unknown as OAuthProviderSettingsResource)

    expect(await screen.findByText('加载 OAuth Provider 设置失败，请重试')).toBeVisible()
    const before = resource.get.mock.calls.length
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(resource.get.mock.calls.length).toBeGreaterThan(before)
  })
})
