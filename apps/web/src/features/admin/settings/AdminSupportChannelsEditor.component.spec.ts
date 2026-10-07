import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import AdminSupportChannelsEditor from './AdminSupportChannelsEditor.vue'
import { SUPPORT_CHANNELS_QUERY_KEY } from '../../../shared/composables/useSupportChannels'

vi.mock('../../../shared/composables/useAppToast')
const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('support update'))
vi.mock('../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

describe('AdminSupportChannelsEditor', () => {
  beforeEach(() => mockAppToast())

  it('persists edited channel state with the audit reason and rejects unsafe URLs', async () => {
    const saved = {
      id: '1',
      name: 'Discord',
      kind: 'discord',
      url: 'https://discord.com/support',
      enabled: true,
      sort_order: 0,
    }
    const update = vi.fn().mockImplementation(async (draft) => draft)
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { user } = renderWithUser(AdminSupportChannelsEditor, {
      props: { resource: { get: vi.fn().mockResolvedValue({ channels: [saved] }), update } },
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    await screen.findByDisplayValue('Discord')
    await user.clear(screen.getByLabelText('渠道链接'))
    await user.type(screen.getByLabelText('渠道链接'), 'https://discord.com/new')
    await user.click(screen.getByRole('switch', { name: '启用渠道 Discord' }))
    await user.click(screen.getByRole('button', { name: '保存' }))
    await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update.mock.calls[0]?.[0].channels[0]).toMatchObject({
      url: 'https://discord.com/new',
      enabled: false,
      sort_order: 0,
    })
    expect(update.mock.calls[0]?.[1]).toBe('support update')
    expect(invalidate).toHaveBeenCalledWith({ queryKey: SUPPORT_CHANNELS_QUERY_KEY })
    expect(queryClient.getQueryData(['admin', 'settings', 'support-channels'])).toMatchObject({
      channels: [{ url: 'https://discord.com/new' }],
    })

    await user.clear(screen.getByLabelText('渠道链接'))
    await user.type(screen.getByLabelText('渠道链接'), 'https://discord.com/unsafe\\path')
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(update).toHaveBeenCalledTimes(1)
  })

  it('adds and removes channels from an empty configuration', async () => {
    const resource = { get: vi.fn().mockResolvedValue({ channels: [] }), update: vi.fn() }
    const { user } = renderWithUser(AdminSupportChannelsEditor, {
      props: { resource },
      global: {
        plugins: [
          [
            VueQueryPlugin,
            { queryClient: new QueryClient({ defaultOptions: { queries: { retry: false } } }) },
          ],
        ],
      },
    })
    expect(await screen.findByText('暂无客服渠道')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '添加渠道' }))
    expect(screen.getByLabelText('渠道名称')).toBeVisible()
    expect(screen.getByLabelText('渠道链接')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '删除渠道' }))
    expect(screen.getByText('暂无客服渠道')).toBeVisible()
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('moves channels and restores their saved ordering', async () => {
    const channels = ['First', 'Second'].map((name, index) => ({
      id: String(index),
      name,
      kind: 'custom',
      url: 'https://example.com',
      enabled: true,
      sort_order: index,
    }))
    const { user } = renderWithUser(AdminSupportChannelsEditor, {
      props: { resource: { get: vi.fn().mockResolvedValue({ channels }), update: vi.fn() } },
      global: {
        plugins: [
          [
            VueQueryPlugin,
            { queryClient: new QueryClient({ defaultOptions: { queries: { retry: false } } }) },
          ],
        ],
      },
    })
    await screen.findByDisplayValue('First')
    await user.click(screen.getAllByRole('button', { name: '下移渠道' })[0]!)
    expect(screen.getAllByLabelText('渠道名称')[0]).toHaveValue('Second')
    await user.click(screen.getByRole('button', { name: '重置' }))
    expect(screen.getAllByLabelText('渠道名称')[0]).toHaveValue('First')
  })
})
