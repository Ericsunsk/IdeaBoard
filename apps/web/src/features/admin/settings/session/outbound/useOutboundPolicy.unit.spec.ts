import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent } from 'vue'
import { useMswHandlers } from '../../../../../test/msw'
import type { AdminOutboundPolicyFeedback } from '../useSettingsFeedback'
import { useOutboundPolicy } from './useOutboundPolicy'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../testing'

type OutboundPolicyLifecycle = ReturnType<typeof useOutboundPolicy>

describe('useOutboundPolicy', () => {
  it('loads the saved policy, tracks edits, and resets to the last synced value', async () => {
    useMswHandlers(...createOutboundPolicyHandlers(createOutboundPolicyMswState()))
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

  it('keeps the saved response when invalidation returns a lagging server read', async () => {
    const invalidationStarted = createGate()
    const releaseInvalidation = createGate()
    let loadCount = 0
    const state = createOutboundPolicyMswState({
      getAllowHosts: async () => {
        loadCount += 1
        if (loadCount === 1) {
          return ['models.internal']
        }
        invalidationStarted.open()
        await releaseInvalidation.promise
        return ['models.internal']
      },
      respondToUpdate: () => ({ allow_hosts: ['draft.internal'] }),
    })
    useMswHandlers(...createOutboundPolicyHandlers(state))
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      })
      harness.lifecycle.form.value.allow_hosts = 'Draft.Internal'

      const saveResult = harness.lifecycle.save()
      await invalidationStarted.promise

      expect(state.updateRequests).toEqual([
        {
          body: { allow_hosts: ['draft.internal'] },
        },
      ])
      expect(harness.feedback.saved).not.toHaveBeenCalled()
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)
      expect(harness.lifecycle.saving.value).toBe(true)
      expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: ['draft.internal'] })
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
      expect(harness.lifecycle.isDirty.value).toBe(false)
      expect(harness.lifecycle.saving.value).toBe(true)
      expect(harness.feedback.saved).not.toHaveBeenCalled()

      releaseInvalidation.open()

      await expect(saveResult).resolves.toBe(true)
      expect(harness.lifecycle.setting.value).toEqual({ allow_hosts: ['draft.internal'] })
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
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
      ...createOutboundPolicyHandlers(
        createOutboundPolicyMswState({
          loadError: { status: 500, body: { code: 500, msg: 'load failed' } },
        }),
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
    const state = createOutboundPolicyMswState({
      updateError: {
        status: 409,
        body: { error: { code: 'idempotency_conflict', message: 'reused', request_id: 'r' } },
      },
    })
    useMswHandlers(...createOutboundPolicyHandlers(state))
    const harness = mountOutboundPolicy()

    try {
      await vi.waitFor(() => {
        expect(harness.lifecycle.form.value.allow_hosts).toBe('models.internal')
      })
      harness.lifecycle.form.value.allow_hosts = 'draft.internal'

      await expect(harness.lifecycle.save()).resolves.toBe(false)

      expect(harness.feedback.saveFailed).toHaveBeenCalledOnce()
      expect(harness.feedback.saveFailed).toHaveBeenCalledWith(
        expect.objectContaining({ message: '操作已提交，请勿重复操作' }),
      )
      expect(harness.feedback.saved).not.toHaveBeenCalled()
      expect(harness.lifecycle.form.value.allow_hosts).toBe('draft.internal')
      expect(harness.lifecycle.isDirty.value).toBe(true)
      expect(harness.lifecycle.saving.value).toBe(false)
      expect(state.loadRequests).toBe(1)
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
