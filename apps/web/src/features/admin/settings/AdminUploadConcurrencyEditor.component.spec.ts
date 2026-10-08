import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { renderWithUser, screen } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import AdminUploadConcurrencyEditor from './AdminUploadConcurrencyEditor.vue'

vi.mock('../../../shared/composables/useAppToast')

describe('AdminUploadConcurrencyEditor', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    mockAppToast()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({
          data: { upload_max_concurrent: 6 },
        }),
      ),
    )
  })

  it('renders upload concurrency card with hydrated setting value', async () => {
    renderWithUser(AdminUploadConcurrencyEditor, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(screen.getByText('上传并发设置')).toBeVisible()
    expect(screen.getByText('上传并发')).toBeVisible()
    expect(screen.getByText('upload_max_concurrent')).toBeVisible()
    expect(await screen.findByDisplayValue('6')).toBeVisible()
    expect(screen.getByRole('button', { name: '保存' })).toBeVisible()
    expect(screen.getByRole('button', { name: '重置' })).toBeVisible()
  })
})
