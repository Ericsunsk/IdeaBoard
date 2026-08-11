import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { useAdminAccessSettings } from './useAdminAccessSettings'
import { useAdminBrandingSettings } from './useAdminBrandingSettings'
import { useAdminMediaUploadSettings } from './useAdminMediaUploadSettings'
import { useAdminSystemSettingsLifecycle } from './useAdminSystemSettingsLifecycle'
import type {
  AdminAccessSettingsFeedback,
  AdminBrandingSettingsFeedback,
  AdminMediaUploadSettingsFeedback,
} from './useAdminSystemSettingsFeedback'

type ExampleSetting = { value: number }

describe('Admin System Settings lifecycle', () => {
  it('keeps one deterministic loading, draft, save, reset, failure, and background-refresh contract', async () => {
    const remote = ref<ExampleSetting | undefined>()
    const loadError = ref<unknown>()
    const loading = ref(true)
    const saving = ref(false)
    const cacheSaved = vi.fn<() => Promise<void>>().mockResolvedValue()
    const feedback = {
      loadFailed: vi.fn(),
      validationFailed: vi.fn(),
      saved: vi.fn(),
      saveFailed: vi.fn(),
    }
    let failNextSave = false
    let resolveSave: ((saved: ExampleSetting) => void) | undefined
    const lifecycle = useAdminSystemSettingsLifecycle({
      defaultValue: { value: 8 },
      source: remote,
      loadError,
      loading,
      saving,
      normalize: (setting) => ({ value: setting?.value ?? 8 }),
      isSame: (left, right) => left.value === right.value,
      validate: (draft) => (draft.value > 0 ? '' : '值必须大于 0'),
      save: (_draft) => {
        saving.value = true
        return new Promise<ExampleSetting>((resolve, reject) => {
          resolveSave = (saved) => {
            saving.value = false
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

    expect(lifecycle.loading.value).toBe(true)
    expect(lifecycle.isDirty.value).toBe(false)

    loading.value = false
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
    expect(lifecycle.saving.value).toBe(true)
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
    expect(lifecycle.saving.value).toBe(true)
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
  it('preserves Branding load, dirty, reset, save, cache refresh, and feedback', async () => {
    const updateRequests: unknown[] = []
    let saved = false
    useMswHandlers(
      http.get('/api/admin/settings/branding', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: saved
            ? { project_name: 'New Brand', logo_url: 'https://cdn.example.com/new.png' }
            : { project_name: 'IdeaBoard', logo_url: '' },
        }),
      ),
      http.put('/api/admin/settings/branding', async ({ request }) => {
        updateRequests.push(await request.json())
        saved = true
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { project_name: 'New Brand', logo_url: 'https://cdn.example.com/new.png' },
        })
      }),
    )
    const feedback = createBrandingFeedback()
    const harness = mountComposable(() => useAdminBrandingSettings({ feedback }))

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
    let registrationEnabled = false
    useMswHandlers(
      http.get('/api/admin/settings/registration', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { enabled: registrationEnabled } }),
      ),
      http.put('/api/admin/settings/registration', async ({ request }) => {
        const body = (await request.json()) as { enabled: boolean }
        registrationRequests.push(body)
        registrationEnabled = body.enabled
        return HttpResponse.json({ code: 0, msg: 'ok', data: body })
      }),
      http.get('/api/admin/settings/rate-limit', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { window_seconds: 60, max_requests: 120 },
        }),
      ),
      http.put('/api/admin/settings/rate-limit', async ({ request }) => {
        const body = await request.json()
        rateLimitRequests.push(body)
        return HttpResponse.json({ code: 0, msg: 'ok', data: body })
      }),
    )
    const feedback = createAccessFeedback()
    const harness = mountComposable(() => useAdminAccessSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.rateLimitForm.value.window_seconds).toBe(60))
      harness.value.rateLimitForm.value.max_requests = 0

      await expect(harness.value.saveRateLimit()).resolves.toBe(false)
      expect(harness.value.rateLimitValidationError.value).toBe('最大请求需为大于 0 的整数')
      expect(feedback.rateLimitValidationFailed).toHaveBeenCalledOnce()
      expect(rateLimitRequests).toEqual([])

      harness.value.resetRateLimit()
      expect(harness.value.rateLimitForm.value.max_requests).toBe(120)
      harness.value.rateLimitForm.value.max_requests = 240
      await expect(harness.value.saveRateLimit()).resolves.toBe(true)
      expect(rateLimitRequests).toEqual([{ window_seconds: 60, max_requests: 240 }])
      expect(feedback.rateLimitSaved).toHaveBeenCalledOnce()

      await harness.value.handleToggleRegistration(true)
      expect(registrationRequests).toEqual([{ enabled: true }])
      expect(harness.value.registrationEnabled.value).toBe(true)
      expect(feedback.registrationUpdated).toHaveBeenCalledWith(true)
    } finally {
      await harness.unmount()
    }
  })

  it('keeps a failed Media Upload save editable and reports feedback', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/media-upload', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { upload_max_concurrent: 8 } }),
      ),
      http.put('/api/admin/settings/media-upload', () =>
        HttpResponse.json({ code: 500, msg: 'save failed' }, { status: 500 }),
      ),
    )
    const feedback = createSettingFeedback() as AdminMediaUploadSettingsFeedback
    const harness = mountComposable(() => useAdminMediaUploadSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.form.value.upload_max_concurrent).toBe(8))
      harness.value.form.value.upload_max_concurrent = 16

      await expect(harness.value.save()).resolves.toBe(false)

      expect(harness.value.form.value.upload_max_concurrent).toBe(16)
      expect(harness.value.isDirty.value).toBe(true)
      expect(feedback.saveFailed).toHaveBeenCalledOnce()
      expect(feedback.saved).not.toHaveBeenCalled()
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
  }
}
