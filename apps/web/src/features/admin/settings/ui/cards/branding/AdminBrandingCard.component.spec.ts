import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../../../test/render'
import { mockAppToast } from '../../../../../../test/toast'
import AdminBrandingCard from './AdminBrandingCard.vue'

vi.mock('../../../../../../core/composables/useAppToast')
vi.mock('../../../../../../core/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: vi.fn(async (file: File) => ({ uploadFile: file })),
}))

const cropDialogStub = {
  props: ['visible', 'busy', 'file'],
  emits: ['confirm', 'update:visible'],
  template: `<section v-if="visible" role="dialog" aria-label="裁剪 Logo">
    <span>{{ file?.name }}</span>
    <button :disabled="busy" @click="$emit('confirm', file)">确认上传</button>
  </section>`,
}

describe('AdminBrandingCard', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    mockAppToast()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({
          data: { project_name: 'IdeaBoard Brand', logo_url: 'https://example.com/logo.png' },
        }),
      ),
    )
  })

  it('renders branding fields with hydrated values', async () => {
    const { container } = renderWithUser(AdminBrandingCard, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(screen.getByText('通用')).toBeVisible()
    expect(await screen.findByDisplayValue('IdeaBoard Brand')).toBeVisible()
    expect(screen.getByDisplayValue('https://example.com/logo.png')).toBeVisible()
    expect(screen.getByRole('button', { name: '保存' })).toBeVisible()
    expect(screen.getByRole('button', { name: '重置' })).toBeVisible()
    // Logo 格式提示必须可见，不能只藏在 button title 里
    expect(screen.getByText('支持 PNG、SVG、JPG，透明底为佳')).toBeVisible()
    // 预览必须与 Sidebar/Login/账户弹窗的真实渲染一致：object-cover + brand mark
    const previewImage = container.querySelector('img')
    expect(previewImage).not.toBeNull()
    expect(previewImage).toHaveClass('object-cover')
    expect(previewImage).not.toHaveClass('object-contain')
    expect(container.querySelector('.ib-brand-mark__logo')).not.toBeNull()
  })

  it('allows clearing the logo url', async () => {
    const { user } = renderWithUser(AdminBrandingCard, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(await screen.findByDisplayValue('https://example.com/logo.png')).toBeVisible()
    const clearBtn = screen.getByRole('button', { name: '清除 Logo' })
    await user.click(clearBtn)

    expect(screen.getByLabelText('Logo 地址')).toHaveValue('')
  })

  it.each([true, false])(
    'closes the crop dialog only after upload succeeds (%s)',
    async (isSuccessful) => {
      const uploadedLogoURL = 'uploads/site/assets/logo_cropped12345678.png'
      useMswHandlers(
        http.post('/api/control/v1/platform/settings/branding/logo', () =>
          isSuccessful
            ? HttpResponse.json({ data: { logo_url: uploadedLogoURL } })
            : HttpResponse.json(
                { error: { code: 'service_unavailable', message: '上传失败' } },
                { status: 503 },
              ),
        ),
      )
      const { user, container } = renderWithUser(AdminBrandingCard, {
        global: {
          plugins: [[VueQueryPlugin, { queryClient }]],
          stubs: { AdminLogoCropDialog: cropDialogStub },
        },
      })
      await screen.findByDisplayValue('IdeaBoard Brand')
      const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]')!
      await user.upload(fileInput, new File(['image'], 'logo.png', { type: 'image/png' }))
      expect(await screen.findByRole('dialog', { name: '裁剪 Logo' })).toBeVisible()

      await user.click(screen.getByRole('button', { name: '确认上传' }))

      await waitFor(() => {
        if (isSuccessful) {
          expect(screen.queryByRole('dialog', { name: '裁剪 Logo' })).not.toBeInTheDocument()
          expect(screen.getByLabelText('Logo 地址')).toHaveValue(uploadedLogoURL)
        } else {
          expect(screen.getByRole('dialog', { name: '裁剪 Logo' })).toHaveTextContent('logo.png')
          expect(screen.getByRole('button', { name: '确认上传' })).toBeEnabled()
          expect(screen.getByLabelText('Logo 地址')).toHaveValue('https://example.com/logo.png')
        }
      })
    },
  )
})
