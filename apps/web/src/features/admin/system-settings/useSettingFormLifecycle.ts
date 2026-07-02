import { computed, watch } from 'vue'
import type { ComputedRef } from 'vue'

type ReadonlyValue<T> = {
  readonly value: T
}

type SettingFormLifecycleFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed?: (message: string) => void
  saved?: () => void
  saveFailed?: (error: unknown) => void
}

type SettingFormSaveAction = {
  validate?: () => string
  run: () => Promise<unknown> | unknown
}

export function useSettingFormLifecycle<TSource, TCurrent = TSource, TSynced = TCurrent>(options: {
  error: ReadonlyValue<unknown>
  source: ReadonlyValue<TSource | null | undefined>
  current: ReadonlyValue<TCurrent>
  synced: ReadonlyValue<TSynced>
  isSaving: ReadonlyValue<boolean>
  feedback: SettingFormLifecycleFeedback
  hydrate: (setting: TSource) => void
  isSame: (current: TCurrent, synced: TSynced) => boolean
  reset: (synced: TSynced) => void
}) {
  const isDirty: ComputedRef<boolean> = computed(
    () => !options.isSame(options.current.value, options.synced.value),
  )

  function getSaveFeedback() {
    const { validationFailed, saved, saveFailed } = options.feedback
    if (!validationFailed || !saved || !saveFailed) {
      throw new Error('Setting form save feedback is not configured')
    }
    return { validationFailed, saved, saveFailed }
  }

  async function save(action: SettingFormSaveAction): Promise<boolean> {
    const feedback = getSaveFeedback()
    const validationError = action.validate?.() ?? ''
    if (validationError) {
      feedback.validationFailed(validationError)
      return false
    }

    try {
      await action.run()
      feedback.saved()
      return true
    } catch (error) {
      feedback.saveFailed(error)
      return false
    }
  }

  function reset() {
    options.reset(options.synced.value)
  }

  watch(
    () => options.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )

  watch(
    () => options.source.value,
    (setting) => {
      if (!setting || options.isSaving.value || isDirty.value) return
      options.hydrate(setting)
    },
    { immediate: true },
  )

  return {
    isDirty,
    reset,
    save,
  }
}
