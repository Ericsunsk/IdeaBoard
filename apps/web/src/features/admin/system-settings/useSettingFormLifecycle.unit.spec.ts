import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { QueryKey } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

type TestSetting = {
  count: number
  title: string
}

type TestSettingResource = {
  form: Ref<TestSetting>
  isDirty: ComputedRef<boolean>
  loading: ComputedRef<boolean>
  reset: () => void
  save: () => Promise<boolean>
  saving: ComputedRef<boolean>
}

const testSettingQueryKey = ['admin', 'settings', 'resource-lifecycle-test'] as const
const extraTestSettingQueryKey = ['site', 'settings', 'resource-lifecycle-test'] as const

describe('useSettingResourceLifecycle', () => {
  it('hydrates clean forms, preserves dirty forms, and resets from the synced setting', async () => {
    const harness = mountSettingResource({ source: { count: 2, title: ' Initial ' } })

    try {
      expect(harness.resource.form.value).toEqual({ count: 2, title: 'Initial' })
      expect(harness.resource.isDirty.value).toBe(false)
      expect(harness.resource.loading.value).toBe(false)

      harness.isFetching.value = true
      expect(harness.resource.loading.value).toBe(true)

      const loadError = new Error('load failed')
      harness.error.value = loadError
      await nextTick()
      expect(harness.feedback.loadFailed).toHaveBeenCalledWith(loadError)

      harness.resource.form.value = { count: 4, title: 'Draft' }
      harness.source.value = { count: 5, title: 'Server' }
      await nextTick()

      expect(harness.resource.form.value).toEqual({ count: 4, title: 'Draft' })
      expect(harness.resource.isDirty.value).toBe(true)

      harness.resource.reset()
      expect(harness.resource.form.value).toEqual({ count: 2, title: 'Initial' })
      expect(harness.resource.isDirty.value).toBe(false)
    } finally {
      await harness.unmount()
    }
  })

  it('saves through the resource seam and updates the query cache before invalidation', async () => {
    const savedSetting = { count: 7, title: ' Saved ' }
    const harness = mountSettingResource({
      save: vi.fn().mockResolvedValue(savedSetting),
    })
    const invalidateSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      harness.resource.form.value = { count: 6.8, title: ' Draft ' }

      await harness.resource.save()

      expect(harness.saveSetting).toHaveBeenCalledWith(
        { count: 6, title: 'Draft' },
        expect.objectContaining({ client: harness.queryClient }),
      )
      expect(harness.resource.form.value).toEqual({ count: 7, title: 'Saved' })
      expect(harness.queryClient.getQueryData(testSettingQueryKey)).toEqual({
        code: 0,
        msg: 'ok',
        data: { count: 7, title: 'Saved' },
      })
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: testSettingQueryKey })
      expect(harness.feedback.saved).toHaveBeenCalledTimes(1)
    } finally {
      await harness.unmount()
    }
  })

  it('stops invalid saves before mutation and cache side effects', async () => {
    const harness = mountSettingResource({
      validate: () => '标题不能为空',
    })
    const invalidateSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      await harness.resource.save()

      expect(harness.feedback.validationFailed).toHaveBeenCalledWith('标题不能为空')
      expect(harness.saveSetting).not.toHaveBeenCalled()
      expect(harness.queryClient.getQueryData(testSettingQueryKey)).toBeUndefined()
      expect(invalidateSpy).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('writes additional query cache and invalidates extra query keys after save', async () => {
    const harness = mountSettingResource({
      extraInvalidateQueryKeys: [extraTestSettingQueryKey],
      writeAdditionalQueryCache: (queryClient, setting) => {
        queryClient.setQueryData(extraTestSettingQueryKey, {
          code: 0,
          msg: 'ok',
          data: setting,
        })
      },
    })
    const invalidateSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      harness.resource.form.value = { count: 3, title: ' Shared ' }

      await harness.resource.save()

      expect(harness.queryClient.getQueryData(extraTestSettingQueryKey)).toEqual({
        code: 0,
        msg: 'ok',
        data: { count: 3, title: 'Shared' },
      })
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: testSettingQueryKey })
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: extraTestSettingQueryKey })
    } finally {
      await harness.unmount()
    }
  })
})

function mountSettingResource(
  options: {
    extraInvalidateQueryKeys?: QueryKey[]
    save?: (setting: TestSetting) => Promise<TestSetting>
    source?: Partial<TestSetting> | null
    validate?: (setting: TestSetting) => string
    writeAdditionalQueryCache?: (queryClient: QueryClient, setting: TestSetting) => void
  } = {},
) {
  let resource: TestSettingResource | null = null
  const source = ref<Partial<TestSetting> | null | undefined>(options.source ?? null)
  const error = ref<unknown>(null)
  const isFetching = ref(false)
  const isLoading = ref(false)
  const feedback = {
    loadFailed: vi.fn(),
    saved: vi.fn(),
    saveFailed: vi.fn(),
    validationFailed: vi.fn(),
  }
  const saveSetting = vi.fn(options.save ?? (async (setting: TestSetting) => setting))
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
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
        resource = useSettingResourceLifecycle<TestSetting>({
          defaultSetting: { count: 0, title: '' },
          extraInvalidateQueryKeys: options.extraInvalidateQueryKeys,
          feedback,
          isSame: (current, synced) =>
            current.count === synced.count && current.title === synced.title,
          normalize: normalizeTestSetting,
          query: {
            error,
            isFetching,
            isLoading,
          },
          queryKey: testSettingQueryKey,
          save: saveSetting,
          source,
          validate: options.validate ?? (() => ''),
          writeAdditionalQueryCache: options.writeAdditionalQueryCache,
        })
        return () => null
      },
    }),
  )

  document.body.appendChild(host)
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)

  const mountedResource = requireMountedSettingResource(resource)

  return {
    error,
    feedback,
    isFetching,
    isLoading,
    queryClient,
    resource: mountedResource,
    saveSetting,
    source,
    async unmount() {
      app.unmount()
      await queryClient.cancelQueries()
      queryClient.clear()
      host.remove()
    },
  }
}

function requireMountedSettingResource(resource: TestSettingResource | null): TestSettingResource {
  if (!resource) throw new Error('expected setting resource to mount')
  return resource
}

function normalizeTestSetting(setting: Partial<TestSetting> | null | undefined): TestSetting {
  return {
    count:
      typeof setting?.count === 'number' && Number.isFinite(setting.count)
        ? Math.trunc(setting.count)
        : 0,
    title: typeof setting?.title === 'string' ? setting.title.trim() : '',
  }
}
