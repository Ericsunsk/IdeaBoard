import { defineComponent } from 'vue'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import SearchProviderCard from './SearchProviderCard.vue'
import * as searchProviderResource from './searchProviderResource'

vi.mock('./searchProviderResource', () => ({
  fetchSearchProviders: vi.fn(),
  saveSearchProvider: vi.fn(),
  testSearchProvider: vi.fn(),
}))

vi.mock('../shared/logos/adminBrandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

const Harness = defineComponent({
  components: { SearchProviderCard },
  template: `<SearchProviderCard />`,
})

describe('SearchProviderCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(searchProviderResource.fetchSearchProviders).mockResolvedValue([
      {
        provider: 'unsplash',
        enabled: true,
        api_key_configured: true,
        test_state: 'passed',
        status_message: '连接正常',
      },
      {
        provider: 'pexels',
        enabled: false,
        api_key_configured: false,
        test_state: 'never',
      },
      {
        provider: 'vision',
        enabled: false,
        api_key_configured: true,
        endpoint_url: 'https://gateway.ai.cloudflare.com',
        model: 'gpt-4o',
        test_state: 'passed',
      },
    ])
  })

  it('renders minimal logo tiles and opens configuration dialog on click', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    expect(await screen.findByText('启用')).toBeInTheDocument()
    const unsplashBtn = await screen.findByRole('button', { name: 'Unsplash' })
    expect(screen.getByRole('button', { name: 'Pexels' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Vision Search' })).toBeInTheDocument()

    // Click on Unsplash card to open dialog
    await user.click(unsplashBtn)

    expect(await screen.findByText('Unsplash')).toBeInTheDocument()
    expect(screen.getByText('素材图库')).toBeInTheDocument()
    expect(screen.getByText('启用该服务')).toBeInTheDocument()
    expect(await screen.findByPlaceholderText('API Key 已配置 (留空保持不变)')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '测试连接' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存配置' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '取消' })).toBeInTheDocument()
  })

  it('renders Vision-specific fields in the dialog', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    const visionBtn = await screen.findByRole('button', { name: 'Vision Search' })
    await user.click(visionBtn)

    expect(await screen.findByText('Vision Search')).toBeInTheDocument()
    expect(screen.getByText('视觉模型')).toBeInTheDocument()
    expect(await screen.findByPlaceholderText('例如: https://gateway.ai.cloudflare.com/v1/...')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('例如: @cf/meta/llama-3.2-11b-vision-instruct 或 gpt-4o')).toBeInTheDocument()
  })

  it('requires saving draft configuration before testing and never saves from the test action', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    await user.click(await screen.findByRole('button', { name: 'Unsplash' }))
    await user.type(
      await screen.findByPlaceholderText('API Key 已配置 (留空保持不变)'),
      'rotated-key',
    )

    expect(screen.getByText('配置已修改，请先保存配置后再测试连接。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '测试连接' })).toBeDisabled()
    expect(searchProviderResource.saveSearchProvider).not.toHaveBeenCalled()
    expect(searchProviderResource.testSearchProvider).not.toHaveBeenCalled()
  })

  it('tests saved configuration without saving or changing the enabled draft', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    await user.click(await screen.findByRole('button', { name: 'Unsplash' }))
    await user.click(screen.getByRole('button', { name: '测试连接' }))

    expect(searchProviderResource.testSearchProvider).toHaveBeenCalledWith('unsplash')
    expect(searchProviderResource.saveSearchProvider).not.toHaveBeenCalled()
  })

  it('discards draft values when the dialog is cancelled', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    await user.click(await screen.findByRole('button', { name: 'Unsplash' }))
    const apiKey = await screen.findByPlaceholderText('API Key 已配置 (留空保持不变)')
    await user.type(apiKey, 'unsaved-key')
    await user.click(screen.getByRole('button', { name: '取消' }))
    await user.click(await screen.findByRole('button', { name: 'Unsplash' }))

    expect(await screen.findByPlaceholderText('API Key 已配置 (留空保持不变)')).toHaveValue('')
    expect(searchProviderResource.saveSearchProvider).not.toHaveBeenCalled()
  })

  it('does not save draft configuration when a connection test fails', async () => {
    vi.mocked(searchProviderResource.testSearchProvider).mockRejectedValueOnce(new Error('连接失败'))
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    await user.click(await screen.findByRole('button', { name: 'Unsplash' }))
    await user.click(screen.getByRole('button', { name: '测试连接' }))

    expect(searchProviderResource.testSearchProvider).toHaveBeenCalledWith('unsplash')
    expect(searchProviderResource.saveSearchProvider).not.toHaveBeenCalled()
  })
})
