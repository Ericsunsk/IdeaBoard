import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import type { AdminOutboundPolicyFeedback } from './useSystemSettingsFeedback'
import { useOutboundPolicy } from './useOutboundPolicy'

type OutboundPolicyLifecycle = ReturnType<typeof useOutboundPolicy>

describe('useOutboundPolicy', () => {
  it('loads the saved policy, tracks edits, and resets to the last synced value', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: ['models.internal'] },
        }),
      ),
    )
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: ['models.internal'] })
      })
      expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)

      harness.lifecycle.form.value.allow_hosts = 'models.internal\n10.0.0.8:8443'

      expect(harness.lifecycle.isDirty.value).toBe(true)

      harness.lifecycle.reset()

      expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)
      expect(harness.feedback.loadFailed).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('reports validation feedback and stops invalid drafts before saving', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { allow_hosts: [] } }),
      ),
    )
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: [] })
      })
      harness.lifecycle.form.value.allow_hosts = 'https://models.internal'

      await expect(harness.lifecycle.save()).resolves.toBe(false)

      expect(harness.lifecycle.validationError.value).toBe(
        '私有主机仅支持 hostname、IP 或 host:port，每行一个',
      )
      expect(harness.feedback.validationFailed).toHaveBeenCalledWith(
        '私有主机仅支持 hostname、IP 或 host:port，每行一个',
      )
      expect(harness.feedback.saved).not.toHaveBeenCalled()
      expect(harness.feedback.saveFailed).not.toHaveBeenCalled()

      harness.lifecycle.form.value.allow_hosts = 'models.internal'
      await nextTick()

      expect(harness.lifecycle.validationError.value).toBe('')
    } finally {
      await harness.unmount()
    }
  })

  it('hydrates the saved response before invalidation and finishes with refreshed server state', async () => {
    const invalidationStarted = createGate()
    const releaseInvalidation = createGate()
    const updateRequests: unknown[] = []
    let loadCount = 0
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', async () => {
        loadCount += 1
        if (loadCount === 1) {
          return HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: { allow_hosts: ['models.internal'] },
          })
        }
        invalidationStarted.open()
        await releaseInvalidation.promise
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: ['server.internal'] },
        })
      }),
      http.put('/api/admin/settings/outbound-policy', async ({ request }) => {
        updateRequests.push(await request.json())
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: ['draft.internal'] },
        })
      }),
    )
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      })
      harness.lifecycle.form.value.allow_hosts = 'Draft.Internal'

      const saveResult = harness.lifecycle.save()
      await invalidationStarted.promise

      expect(updateRequests).toEqual([{ allow_hosts: ['draft.internal'] }])
      expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: ['draft.internal'] })
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)
      expect(harness.lifecycle.saving.value).toBe(true)
      expect(harness.feedback.saved).not.toHaveBeenCalled()

      releaseInvalidation.open()

      await expect(saveResult).resolves.toBe(true)
      expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: ['server.internal'] })
      expect(harness.lifecycle.form.value.allow_hosts).toBe('server.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)
      expect(harness.lifecycle.saving.value).toBe(false)
      expect(harness.feedback.saved).toHaveBeenCalledTimes(1)
    } finally {
      releaseInvalidation.open()
      await harness.unmount()
    }
  })

  it('surfaces load errors through state and feedback', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({ code: 500, msg: 'load failed' }, { status: 500 }),
      ),
    )
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.error.value).toBeTruthy()
      })

      expect(harness.feedback.loadFailed).toHaveBeenCalledWith(harness.lifecycle.error.value)
      expect(harness.lifecycle.loading.value).toBe(false)
    } finally {
      await harness.unmount()
    }
  })

  it('keeps a failed save editable and reports save feedback', async () => {
    let loadCount = 0
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', () => {
        loadCount += 1
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: ['models.internal'] },
        })
      }),
      http.put('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({ code: 500, msg: 'save failed' }, { status: 500 }),
      ),
    )
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      })
      harness.lifecycle.form.value.allow_hosts = 'draft.internal'

      await expect(harness.lifecycle.save()).resolves.toBe(false)

      expect(harness.feedback.saveFailed).toHaveBeenCalledOnce()
      expect(harness.feedback.saved).not.toHaveBeenCalled()
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
      expect(harness.lifecycle.isDirty.value).toBe(true)
      expect(harness.lifecycle.saving.value).toBe(false)
      expect(loadCount).toBe(1)
    } finally {
      await harness.unmount()
    }
  })
})

function mountOutboundPolicy() {
  let lifecycle: OutboundPolicyLifecycle | null = null
  const feedback = createFeedback()
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: {
        gcTime: 0,
        refetchOnWindowFocus: false,
        retry: false,
      },
    },
  })
  const host = document.createElement('div')
  const app = createApp(
    defineComponent({
      setup() {
        lifecycle = useOutboundPolicy({ feedback })
        return () => null
      },
    }),
  )

  document.body.appendChild(host)
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)

  return {
    feedback,
    lifecycle: requireMountedLifecycle(lifecycle),
    async unmount() {
      app.unmount()
      await queryClient.cancelQueries()
      queryClient.clear()
      host.remove()
    },
  }
}

function createFeedback(): AdminOutboundPolicyFeedback {
  return {
    loadFailed: vi.fn(),
    saved: vi.fn(),
    saveFailed: vi.fn(),
    validationFailed: vi.fn(),
  }
}

function requireMountedLifecycle(
  lifecycle: OutboundPolicyLifecycle | null,
): OutboundPolicyLifecycle {
  if (!lifecycle) throw new Error('expected outbound policy lifecycle to mount')
  return lifecycle
}

function createGate() {
  let open = () => {}
  const promise = new Promise<void>((resolve) => {
    open = resolve
  })
  return { open, promise }
}
