import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { SETTINGS_MESSAGES } from '../pages/messages'
import AdminUploadConcurrencyFooter from './AdminUploadConcurrencyFooter.vue'

vi.mock('../../../../core/composables/useAppToast')

describe('AdminUploadConcurrencyFooter', () => {
  beforeEach(() => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({
          data: { upload_max_concurrent: 6 },
        }),
      ),
    )
  })

  function renderFooter() {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return renderWithUser(AdminUploadConcurrencyFooter, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })
  }

  it('renders the concurrency row with the hydrated setting value', async () => {
    renderFooter()

    expect(screen.getByText(SETTINGS_MESSAGES.uploadRowTitle)).toBeVisible()
    expect(await screen.findByDisplayValue('6')).toBeVisible()
    expect(screen.getByRole('button', { name: '保存' })).toBeVisible()
    expect(screen.getByRole('button', { name: '重置' })).toBeVisible()
  })

  it('saves an edited concurrency value without a confirm dialog', async () => {
    const updateRequests: unknown[] = []
    let currentConcurrent = 6
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({
          data: { upload_max_concurrent: currentConcurrent },
        }),
      ),
      http.put('/api/control/v1/platform/settings/media-upload', async ({ request }) => {
        const body = await request.json()
        currentConcurrent = (body as { upload_max_concurrent: number }).upload_max_concurrent
        updateRequests.push(body)
        return HttpResponse.json({ data: body })
      }),
    )
    const { user } = renderFooter()

    const input = await screen.findByDisplayValue('6')
    await user.clear(input)
    await user.type(input, '12')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await vi.waitFor(() => expect(updateRequests).toHaveLength(1))
    expect(updateRequests[0]).toEqual({ upload_max_concurrent: 12 })
    expect(await screen.findByDisplayValue('12')).toBeVisible()
  })

  it('routes validation failure to toast error without an inline alert', async () => {
    const toast = mockAppToast()
    const { user } = renderFooter()

    const input = await screen.findByDisplayValue('6')
    await user.clear(input)
    await user.type(input, '0')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await vi.waitFor(() => expect(toast.error).toHaveBeenCalled())
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('1 到 64'))
    expect(toast.success).not.toHaveBeenCalled()
    // 精简目标：内嵌校验文案已删除，失败一律 toast；行标题仍为真相源。
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText(SETTINGS_MESSAGES.uploadRowTitle)).toBeVisible()
  })
})
