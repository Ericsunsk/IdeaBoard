import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { useAccessSettings } from './access/useAccessSettings'
import { useBrandingSettings } from './branding/useBrandingSettings'
import { useMediaUploadSettings } from './upload/useMediaUploadSettings'
import { useSettingsLifecycle } from './useSettingsLifecycle'
import type {
  AdminAccessSettingsFeedback,
  AdminBrandingSettingsFeedback,
  AdminMediaUploadSettingsFeedback,
} from './useSettingsFeedback'

type ExampleSetting = { value: number }

describe('Admin System Settings lifecycle', () => {
  it('keeps one deterministic loading, draft, save, reset, failure, and background-refresh contract', async () => {
    const remote = ref<ExampleSetting | undefined>()
    const loadError = ref<unknown>()
    const isLoading = ref(true)
    const isSaving = ref(false)
    const cacheSaved = vi.fn<() => Promise<void>>().mockResolvedValue()
    const feedback = {
      loadFailed: vi.fn(),
      validationFailed: vi.fn(),
      saved: vi.fn(),
      saveFailed: vi.fn(),
    }
    let failNextSave = false
    let resolveSave: ((saved: ExampleSetting) => void) | undefined
    const lifecycle = useSettingsLifecycle({
      defaultValue: { value: 8 },
      source: remote,
      loadError,
      isLoading,
      isSaving,
      normalize: (setting) => ({ value: setting?.value ?? 8 }),
      isSame: (left, right) => left.value === right.value,
      validate: (draft) => (draft.value > 0 ? '' : '值必须大于 0'),
      save: (_draft) => {
        isSaving.value = true
        return new Promise<ExampleSetting>((resolve, reject) => {
          resolveSave = (saved) => {
            isSaving.value = false
            if (failNextSave) {
              reject(new Error('save failed'))
              return
            }
            resolve(saved)
          }
        })
      },
      synchronizeCache: cacheSaved,
      feedback,
    })

    expect(lifecycle.isLoading.value).toBe(true)
    expect(lifecycle.isDirty.value).toBe(false)

    isLoading.value = false
    remote.value = { value: 12 }
    await nextTick()
    expect(lifecycle.form.value).toEqual({ value: 12 })
    expect(lifecycle.isDirty.value).toBe(false)

    lifecycle.form.value.value = 16
    expect(lifecycle.isDirty.value).toBe(true)
    remote.value = { value: 20 }
    await nextTick()
    expect(lifecycle.form.value).toEqual({ value: 16 })

    const saveResult = lifecycle.save()
    expect(lifecycle.isSaving.value).toBe(true)
    resolveSave?.({ value: 16 })
    await expect(saveResult).resolves.toBe(true)
    expect(cacheSaved).toHaveBeenCalledWith({ value: 16 })
    expect(lifecycle.form.value).toEqual({ value: 16 })
    expect(lifecycle.isDirty.value).toBe(false)
    expect(feedback.saved).toHaveBeenCalledOnce()

    lifecycle.form.value.value = 0
    await expect(lifecycle.save()).resolves.toBe(false)
    expect(lifecycle.validationError.value).toBe('值必须大于 0')
    expect(feedback.validationFailed).toHaveBeenCalledWith('值必须大于 0')

    lifecycle.form.value.value = 24
    failNextSave = true
    const failedSaveResult = lifecycle.save()
    expect(lifecycle.isSaving.value).toBe(true)
    resolveSave?.({ value: 24 })
    await expect(failedSaveResult).resolves.toBe(false)
    expect(lifecycle.form.value).toEqual({ value: 24 })
    expect(lifecycle.isDirty.value).toBe(true)
    expect(feedback.saveFailed).toHaveBeenCalledOnce()

    lifecycle.reset()
    expect(lifecycle.form.value).toEqual({ value: 16 })
    expect(lifecycle.isDirty.value).toBe(false)

    remote.value = { value: 32 }
    await nextTick()
    expect(lifecycle.form.value).toEqual({ value: 32 })

    loadError.value = new Error('load failed')
    await nextTick()
    expect(feedback.loadFailed).toHaveBeenCalledOnce()
  })
})

describe('Admin System Settings adapters', () => {
  it('uploads a branding logo as multipart with an idempotency key and updates only the draft', async () => {
    const uploadedLogoKey = 'uploads/site/assets/logo_abcdef1234567890.png'
    let uploadBody: FormData | undefined
    let idempotencyKey: string | null = null
    const saveRequest = vi.fn()
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({ data: { project_name: 'IdeaBoard', logo_url: '' } }),
      ),
      http.post('/api/control/v1/platform/settings/branding/logo', async ({ request }) => {
        uploadBody = await request.formData()
        idempotencyKey = request.headers.get('Idempotency-Key')
        return HttpResponse.json({ data: { logo_url: uploadedLogoKey } })
      }),
      http.put('/api/control/v1/platform/settings/branding', saveRequest),
    )
    const feedback = createBrandingFeedback()
    const harness = mountComposable(() => useBrandingSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.isLoading.value).toBe(false))
      harness.value.form.value.project_name = 'Draft Brand'
      const file = new File(['logo contents'], 'logo.png', { type: 'image/png' })

      await expect(harness.value.handleUploadLogo(file)).resolves.toBe(true)

      const uploadedFile = uploadBody?.get('file') as File
      expect(uploadedFile.name).toBe('logo.png')
      expect(uploadedFile.type).toBe('image/png')
      expect(await uploadedFile.text()).toBe('logo contents')
      expect(idempotencyKey).toBeTruthy()
      expect(harness.value.form.value).toEqual({
        project_name: 'Draft Brand',
        logo_url: uploadedLogoKey,
      })
      expect(harness.value.logoPreviewURL.value).toBe(
        `/api/v1/public/branding/logo?key=${encodeURIComponent(uploadedLogoKey)}`,
      )
      expect(harness.value.isDirty.value).toBe(true)
      expect(harness.value.uploading.value).toBe(false)
      expect(feedback.logoUploaded).toHaveBeenCalledOnce()
      expect(feedback.logoUploadFailed).not.toHaveBeenCalled()
      expect(saveRequest).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('preserves the branding draft and reports a failed logo upload', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({ data: { project_name: 'IdeaBoard', logo_url: '' } }),
      ),
      http.post('/api/control/v1/platform/settings/branding/logo', () =>
        HttpResponse.json(
          { error: { code: 'payload_too_large', message: 'Logo 文件过大' } },
          { status: 413 },
        ),
      ),
    )
    const feedback = createBrandingFeedback()
    const harness = mountComposable(() => useBrandingSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.isLoading.value).toBe(false))
      const draft = { project_name: 'Draft Brand', logo_url: 'https://example.com/original.png' }
      harness.value.form.value = { ...draft }

      await expect(
        harness.value.handleUploadLogo(new File(['logo'], 'logo.png', { type: 'image/png' })),
      ).resolves.toBe(false)

      expect(harness.value.form.value).toEqual(draft)
      expect(harness.value.uploading.value).toBe(false)
      expect(feedback.logoUploadFailed).toHaveBeenCalledWith(
        expect.objectContaining({ message: 'Logo 文件过大' }),
      )
      expect(feedback.logoUploaded).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('preserves Branding load, dirty, reset, save, cache refresh, and feedback', async () => {
    const updateRequests: unknown[] = []
    let saved = false
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({
          data: saved
            ? { project_name: 'New Brand', logo_url: 'https://cdn.example.com/new.png' }
            : { project_name: 'IdeaBoard', logo_url: '' },
        }),
      ),
      http.put('/api/control/v1/platform/settings/branding', async ({ request }) => {
        updateRequests.push(await request.json())
        saved = true
        return HttpResponse.json({
          data: { project_name: 'New Brand', logo_url: 'https://cdn.example.com/new.png' },
        })
      }),
    )
    const feedback = createBrandingFeedback()
    const harness = mountComposable(() => useBrandingSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.form.value.project_name).toBe('IdeaBoard'))
      harness.value.form.value.project_name = 'Draft Brand'
      expect(harness.value.isDirty.value).toBe(true)

      harness.value.reset()
      expect(harness.value.form.value.project_name).toBe('IdeaBoard')
      expect(harness.value.isDirty.value).toBe(false)

      harness.value.form.value.project_name = ' New Brand '
      harness.value.form.value.logo_url = ' https://cdn.example.com/new.png '
      await expect(harness.value.save()).resolves.toBe(true)

      expect(updateRequests).toEqual([
        { project_name: 'New Brand', logo_url: 'https://cdn.example.com/new.png' },
      ])
      expect(harness.value.form.value.project_name).toBe('New Brand')
      expect(harness.value.isDirty.value).toBe(false)
      expect(feedback.saved).toHaveBeenCalledOnce()
      expect(feedback.loadFailed).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('preserves Access rate-limit validation/save and registration toggle sequencing', async () => {
    const rateLimitRequests: unknown[] = []
    const registrationRequests: unknown[] = []
    const rateLimitHeaders: Array<{
      idempotencyKey: string | null
    }> = []
    const registrationHeaders: Array<{
      idempotencyKey: string | null
    }> = []
    let registrationEnabled = false
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/registration', () =>
        HttpResponse.json({ data: { enabled: registrationEnabled } }),
      ),
      http.put('/api/control/v1/platform/settings/registration', async ({ request }) => {
        const body = (await request.json()) as { enabled: boolean }
        registrationRequests.push(body)
        registrationHeaders.push({
          idempotencyKey: request.headers.get('Idempotency-Key'),
        })
        registrationEnabled = body.enabled
        return HttpResponse.json({ data: body })
      }),
      http.get('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json({
          data: { window_seconds: 60, max_requests: 120 },
        }),
      ),
      http.get('/api/control/v1/platform/settings/login-lockout', () =>
        HttpResponse.json({
          data: { threshold: 10, duration_seconds: 900 },
        }),
      ),
      http.put('/api/control/v1/platform/settings/rate-limit', async ({ request }) => {
        const body = await request.json()
        rateLimitRequests.push(body)
        rateLimitHeaders.push({
          idempotencyKey: request.headers.get('Idempotency-Key'),
        })
        return HttpResponse.json({ data: body })
      }),
    )
    const feedback = createAccessFeedback()
    const harness = mountComposable(() => useAccessSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.rateLimitForm.value.window_seconds).toBe(60))
      harness.value.rateLimitForm.value.max_requests = null

      await expect(harness.value.saveRateLimit()).resolves.toBe(false)
      expect(harness.value.rateLimitValidationError.value).toBe('最大请求需为大于 0 的整数')
      expect(feedback.rateLimitValidationFailed).toHaveBeenCalledOnce()
      expect(rateLimitRequests).toEqual([])

      harness.value.resetRateLimit()
      expect(harness.value.rateLimitForm.value.max_requests).toBe(120)
      harness.value.rateLimitForm.value.max_requests = 240
      await expect(harness.value.saveRateLimit()).resolves.toBe(true)
      expect(rateLimitRequests).toEqual([{ window_seconds: 60, max_requests: 240 }])
      expect(rateLimitHeaders[0]?.idempotencyKey).toMatch(/.+/)
      expect(feedback.rateLimitSaved).toHaveBeenCalledOnce()

      await harness.value.handleToggleRegistration(true)
      expect(registrationRequests).toEqual([{ enabled: true }])
      expect(registrationHeaders[0]?.idempotencyKey).toMatch(/.+/)
      expect(harness.value.registrationEnabled.value).toBe(true)
      expect(feedback.registrationUpdated).toHaveBeenCalledWith(true)
    } finally {
      await harness.unmount()
    }
  })

  it('maps Access control errors while keeping the draft editable', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/registration', () =>
        HttpResponse.json({ data: { enabled: false } }),
      ),
      http.get('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json({ data: { window_seconds: 60, max_requests: 120 } }),
      ),
      http.get('/api/control/v1/platform/settings/login-lockout', () =>
        HttpResponse.json({ data: { threshold: 10, duration_seconds: 900 } }),
      ),
      http.put('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json(
          { error: { code: 'idempotency_conflict', message: 'duplicate', request_id: 'req-1' } },
          { status: 409 },
        ),
      ),
    )
    const feedback = createAccessFeedback()
    const harness = mountComposable(() => useAccessSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.rateLimitForm.value.window_seconds).toBe(60))
      harness.value.rateLimitForm.value.max_requests = 240

      await expect(harness.value.saveRateLimit()).resolves.toBe(false)
      expect(feedback.rateLimitSaveFailed).toHaveBeenCalledOnce()
      expect(feedback.rateLimitSaveFailed).toHaveBeenCalledWith(
        expect.objectContaining({
          code: 'idempotency_conflict',
          message: '操作已提交，请勿重复操作',
        }),
      )
      expect(harness.value.rateLimitForm.value.max_requests).toBe(240)
      expect(harness.value.rateLimitDirty.value).toBe(true)
    } finally {
      await harness.unmount()
    }
  })

  it('keeps a failed Media Upload save editable and reports feedback', async () => {
    const updateRequests: unknown[] = []
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({ data: { upload_max_concurrent: 8 } }),
      ),
      http.put('/api/control/v1/platform/settings/media-upload', async ({ request }) => {
        updateRequests.push(await request.json())
        return HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'save failed' } },
          { status: 500 },
        )
      }),
    )
    const feedback = createSettingFeedback() as AdminMediaUploadSettingsFeedback
    const harness = mountComposable(() => useMediaUploadSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.form.value.upload_max_concurrent).toBe(8))

      harness.value.form.value.upload_max_concurrent = '' as unknown as number
      await expect(harness.value.save()).resolves.toBe(false)

      expect(harness.value.validationError.value).toBe('上传并发上限必须在 1 到 64 之间')
      expect(feedback.validationFailed).toHaveBeenCalledOnce()
      expect(updateRequests).toEqual([])

      harness.value.form.value.upload_max_concurrent = 16

      await expect(harness.value.save()).resolves.toBe(false)

      expect(harness.value.form.value.upload_max_concurrent).toBe(16)
      expect(harness.value.isDirty.value).toBe(true)
      expect(feedback.saveFailed).toHaveBeenCalledOnce()
      expect(feedback.saved).not.toHaveBeenCalled()
      expect(updateRequests).toEqual([{ upload_max_concurrent: 16 }])
    } finally {
      await harness.unmount()
    }
  })
})

function mountComposable<T>(setupComposable: () => T) {
  let value: T | null = null
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { gcTime: 0, refetchOnWindowFocus: false, retry: false },
    },
  })
  const host = document.createElement('div')
  const app = createApp(
    defineComponent({
      setup() {
        value = setupComposable()
        return () => null
      },
    }),
  )
  document.body.appendChild(host)
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)

  return {
    value: requireMountedValue<T>(value),
    async unmount() {
      app.unmount()
      await queryClient.cancelQueries()
      queryClient.clear()
      host.remove()
    },
  }
}

function requireMountedValue<T>(value: T | null): T {
  if (!value) throw new Error('expected system settings lifecycle to mount')
  return value
}

function createSettingFeedback() {
  return {
    loadFailed: vi.fn(),
    validationFailed: vi.fn(),
    saved: vi.fn(),
    saveFailed: vi.fn(),
  }
}

function createBrandingFeedback(): AdminBrandingSettingsFeedback {
  return {
    ...createSettingFeedback(),
    logoUploaded: vi.fn(),
    logoUploadFailed: vi.fn(),
  }
}

function createAccessFeedback(): AdminAccessSettingsFeedback {
  return {
    loadFailed: vi.fn(),
    rateLimitValidationFailed: vi.fn(),
    rateLimitSaved: vi.fn(),
    rateLimitSaveFailed: vi.fn(),
    registrationUpdated: vi.fn(),
    registrationUpdateFailed: vi.fn(),
    loginLockoutValidationFailed: vi.fn(),
    loginLockoutSaved: vi.fn(),
    loginLockoutSaveFailed: vi.fn(),
  }
}
