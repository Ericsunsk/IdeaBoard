import { describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useSettingFormLifecycle } from './useSettingFormLifecycle'

describe('useSettingFormLifecycle', () => {
  it('hydrates the form from clean source changes and reports load failures', async () => {
    const scope = effectScope()
    const error = ref<unknown>(null)
    const source = ref<{ title: string } | null>({ title: 'Initial' })
    const isSaving = ref(false)
    const current = ref({ title: '' })
    const synced = ref({ title: '' })
    const loadFailed = vi.fn()
    const hydrate = vi.fn()

    scope.run(() => {
      useSettingFormLifecycle({
        error,
        source,
        current,
        synced,
        isSaving,
        feedback: { loadFailed },
        hydrate,
        isSame: (a, b) => a.title === b.title,
        reset: vi.fn(),
      })
    })

    expect(hydrate).toHaveBeenCalledWith({ title: 'Initial' })

    source.value = { title: 'Updated' }
    await nextTick()
    expect(hydrate).toHaveBeenLastCalledWith({ title: 'Updated' })

    const loadError = new Error('load failed')
    error.value = loadError
    await nextTick()
    expect(loadFailed).toHaveBeenCalledWith(loadError)

    scope.stop()
  })

  it('does not overwrite dirty or saving forms from source changes', async () => {
    const scope = effectScope()
    const source = ref<{ title: string } | null>(null)
    const isSaving = ref(false)
    const current = ref({ title: 'same' })
    const synced = ref({ title: 'same' })
    const hydrate = vi.fn()

    const lifecycle = scope.run(() =>
      useSettingFormLifecycle({
        error: ref(null),
        source,
        current,
        synced,
        isSaving,
        feedback: { loadFailed: vi.fn() },
        hydrate,
        isSame: (a, b) => a.title === b.title,
        reset: vi.fn(),
      }),
    )
    if (!lifecycle) throw new Error('missing lifecycle')

    source.value = { title: 'Clean' }
    await nextTick()
    expect(hydrate).toHaveBeenCalledWith({ title: 'Clean' })
    expect(lifecycle.isDirty.value).toBe(false)

    current.value = { title: 'dirty' }
    source.value = { title: 'Dirty should stay' }
    await nextTick()
    expect(hydrate).not.toHaveBeenCalledWith({ title: 'Dirty should stay' })
    expect(lifecycle.isDirty.value).toBe(true)

    current.value = { title: 'same' }
    isSaving.value = true
    source.value = { title: 'Saving should stay' }
    await nextTick()
    expect(hydrate).not.toHaveBeenCalledWith({ title: 'Saving should stay' })

    scope.stop()
  })

  it('runs save feedback and reset commands through the lifecycle', async () => {
    const scope = effectScope()
    const validationFailed = vi.fn()
    const saved = vi.fn()
    const saveFailed = vi.fn()
    const run = vi.fn()
    const lifecycle = scope.run(() =>
      useSettingFormLifecycle({
        error: ref(null),
        source: ref(null),
        current: ref({ title: 'current' }),
        synced: ref({ title: 'synced' }),
        isSaving: ref(false),
        feedback: {
          loadFailed: vi.fn(),
          validationFailed,
          saved,
          saveFailed,
        },
        hydrate: vi.fn(),
        isSame: (a, b) => a.title === b.title,
        reset: vi.fn(),
      }),
    )
    if (!lifecycle) throw new Error('missing lifecycle')

    await lifecycle.save({
      validate: () => '配置不合法',
      run,
    })
    expect(validationFailed).toHaveBeenCalledWith('配置不合法')
    expect(run).not.toHaveBeenCalled()

    await lifecycle.save({ run })
    expect(run).toHaveBeenCalledTimes(1)
    expect(saved).toHaveBeenCalledTimes(1)

    const error = new Error('save failed')
    await lifecycle.save({
      run: async () => {
        throw error
      },
    })
    expect(saveFailed).toHaveBeenCalledWith(error)

    const reset = vi.fn()
    const resetLifecycle = scope.run(() =>
      useSettingFormLifecycle({
        error: ref(null),
        source: ref(null),
        current: ref({ title: 'current' }),
        synced: ref({ title: 'synced' }),
        isSaving: ref(false),
        feedback: { loadFailed: vi.fn() },
        hydrate: vi.fn(),
        isSame: (a, b) => a.title === b.title,
        reset,
      }),
    )
    if (!resetLifecycle) throw new Error('missing reset lifecycle')
    resetLifecycle.reset()
    expect(reset).toHaveBeenCalledWith({ title: 'synced' })

    scope.stop()
  })
})
