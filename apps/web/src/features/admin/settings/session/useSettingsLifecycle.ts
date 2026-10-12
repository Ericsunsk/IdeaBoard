import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'

type AdminSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

type AdminSettingsLifecycleOptions<T> = {
  defaultValue: T
  source: Readonly<Ref<Partial<T> | null | undefined>>
  loadError: Readonly<Ref<unknown>>
  isLoading: Readonly<Ref<boolean>>
  isSaving: Readonly<Ref<boolean>>
  normalize: (setting: Partial<T> | null | undefined) => T
  isSame: (current: T, synced: T) => boolean
  validate: (draft: T) => string
  save: (draft: T) => Promise<T>
  synchronizeCache: (saved: T) => Promise<void>
  feedback: AdminSettingsFeedback
}

export function useSettingsLifecycle<T>(options: AdminSettingsLifecycleOptions<T>) {
  const form = ref(clone(options.defaultValue)) as Ref<T>
  const lastSyncedForm = ref(clone(options.defaultValue)) as Ref<T>
  const validationError = ref('')
  const isDirty = computed(() => !options.isSame(form.value, lastSyncedForm.value))

  function hydrate(setting: Partial<T> | null | undefined) {
    const nextForm = options.normalize(setting)
    lastSyncedForm.value = clone(nextForm)
    form.value = clone(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const validationMessage = options.validate(form.value)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }

    const normalizedDraft = options.normalize(form.value)

    try {
      const saved = await options.save(normalizedDraft)
      hydrate(saved)
      await options.synchronizeCache(saved)
      options.feedback.saved()
      return true
    } catch (error) {
      options.feedback.saveFailed(error)
      return false
    }
  }

  function reset() {
    form.value = clone(lastSyncedForm.value)
    validationError.value = ''
  }

  watch(form, () => (validationError.value = ''), { deep: true, flush: 'sync' })
  watch(options.loadError, (error) => {
    if (error) options.feedback.loadFailed(error)
  })
  watch(
    options.source,
    (setting) => {
      if (!setting || options.isSaving.value || isDirty.value) return
      hydrate(setting)
    },
    { immediate: true },
  )

  return {
    form,
    isDirty,
    isLoading: computed(() => options.isLoading.value),
    reset,
    save,
    isSaving: computed(() => options.isSaving.value),
    validationError,
  }
}

function clone<T>(value: T): T {
  return structuredClone(toRaw(value))
}
