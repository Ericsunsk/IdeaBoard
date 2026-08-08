import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { useAdminAccessSettings } from './useAdminAccessSettings'
import { useAdminBrandingSettings } from './useAdminBrandingSettings'
import { useAdminMediaUploadSettings } from './useAdminMediaUploadSettings'
import { useAdminTaskMonitorSettings } from './useAdminTaskMonitorSettings'
import type {
  AdminAccessSettingsFeedback,
  AdminBrandingSettingsFeedback,
  AdminMediaUploadSettingsFeedback,
  AdminTaskMonitorSettingsFeedback,
} from './useAdminSystemSettingsFeedback'

describe('admin system settings vertical lifecycles', () => {
  it('owns Branding load, dirty, reset, save, cache refresh, and feedback', async () => {
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

  it('owns Access rate-limit validation/save and registration toggle sequencing', async () => {
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

  it('owns Task Monitor load, validation, reset, save, and feedback', async () => {
    const updateRequests: unknown[] = []
    let pageSize = 20
    useMswHandlers(
      http.get('/api/admin/settings/task-monitor', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            list_page_size: pageSize,
            auto_refresh_enabled: true,
            refresh_interval_seconds: 5,
            result_preview_enabled: true,
          },
        }),
      ),
      http.put('/api/admin/settings/task-monitor', async ({ request }) => {
        const body = (await request.json()) as { list_page_size: number }
        updateRequests.push(body)
        pageSize = body.list_page_size
        return HttpResponse.json({ code: 0, msg: 'ok', data: body })
      }),
    )
    const feedback = createSettingFeedback() as AdminTaskMonitorSettingsFeedback
    const harness = mountComposable(() => useAdminTaskMonitorSettings({ feedback }))

    try {
      await vi.waitFor(() => expect(harness.value.form.value.list_page_size).toBe(20))
      harness.value.form.value.list_page_size = 100
      await expect(harness.value.save()).resolves.toBe(false)
      expect(feedback.validationFailed).toHaveBeenCalledOnce()

      harness.value.reset()
      expect(harness.value.form.value.list_page_size).toBe(20)
      harness.value.form.value.list_page_size = 30
      await expect(harness.value.save()).resolves.toBe(true)
      expect(updateRequests).toHaveLength(1)
      expect(harness.value.form.value.list_page_size).toBe(30)
      expect(feedback.saved).toHaveBeenCalledOnce()
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
