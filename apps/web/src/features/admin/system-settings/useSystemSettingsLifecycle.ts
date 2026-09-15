import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'

type AdminSystemSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

type AdminSystemSettingsLifecycleOptions<T> = {
  defaultValue: T
  source: Readonly<Ref<Partial<T> | null | undefined>>
  loadError: Readonly<Ref<unknown>>
  loading: Readonly<Ref<boolean>>
  saving: Readonly<Ref<boolean>>
  normalize: (setting: Partial<T> | null | undefined) => T
  isSame: (current: T, synced: T) => boolean
  validate: (draft: T) => string
  save: (draft: T) => Promise<T>
  synchronizeCache: (saved: T) => Promise<void>
  feedback: AdminSystemSettingsFeedback
}

export function useSystemSettingsLifecycle<T>(options: AdminSystemSettingsLifecycleOptions<T>) {
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
    const normalizedDraft = options.normalize(form.value)
    const validationMessage = options.validate(normalizedDraft)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }

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
      if (!setting || options.saving.value || isDirty.value) return
      hydrate(setting)
    },
    { immediate: true },
  )

  return {
    form,
    isDirty,
    loading: computed(() => options.loading.value),
    reset,
    save,
    saving: computed(() => options.saving.value),
    validationError,
  }
}

function clone<T>(value: T): T {
  return structuredClone(toRaw(value))
}
