import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryKey } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import type { ComputedRef, Ref } from 'vue'

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

type SettingResourceQuery = {
  error: ReadonlyValue<unknown>
  isFetching: ReadonlyValue<boolean>
  isLoading: ReadonlyValue<boolean>
}

type SettingQueryCacheEntry<TSetting> = {
  code?: number
  msg?: string
  data?: TSetting
}

export function useSettingResourceLifecycle<TSetting extends object>(options: {
  defaultSetting: TSetting
  feedback: SettingFormLifecycleFeedback
  isSame: (current: TSetting, synced: TSetting) => boolean
  normalize: (setting: Partial<TSetting> | null | undefined) => TSetting
  query: SettingResourceQuery
  queryKey: QueryKey
  save: (setting: TSetting) => Promise<TSetting>
  source: ReadonlyValue<Partial<TSetting> | null | undefined>
  validate: (setting: TSetting) => string
}) {
  const queryClient = useQueryClient()
  const form = ref<TSetting>(cloneSetting(options.defaultSetting)) as Ref<TSetting>
  const lastSyncedForm = ref<TSetting>(cloneSetting(options.defaultSetting)) as Ref<TSetting>

  function invalidateQuery() {
    return queryClient.invalidateQueries({ queryKey: options.queryKey })
  }

  function writeQueryCache(setting: TSetting) {
    queryClient.setQueryData<SettingQueryCacheEntry<TSetting>>(options.queryKey, (previous) => ({
      code: previous?.code ?? 0,
      msg: previous?.msg ?? 'ok',
      data: setting,
    }))
  }

  function hydrate(setting: Partial<TSetting> | null | undefined) {
    const normalizedSetting = options.normalize(setting)
    lastSyncedForm.value = cloneSetting(normalizedSetting)
    form.value = cloneSetting(normalizedSetting)
    return normalizedSetting
  }

  const saveMutation = useMutation({
    mutationFn: options.save,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrate(setting)
      writeQueryCache(normalizedSetting)
      await invalidateQuery()
    },
  })

  const loading = computed(() => options.query.isLoading.value || options.query.isFetching.value)
  const saving = computed(() => saveMutation.isPending.value)

  const formLifecycle = useSettingFormLifecycle<Partial<TSetting>, TSetting>({
    error: options.query.error,
    source: options.source,
    current: form,
    synced: lastSyncedForm,
    isSaving: saveMutation.isPending,
    feedback: options.feedback,
    hydrate,
    isSame: options.isSame,
    reset: (synced) => {
      form.value = cloneSetting(synced)
    },
  })

  async function save(): Promise<boolean> {
    return formLifecycle.save({
      validate: () => options.validate(form.value),
      run: () => saveMutation.mutateAsync(options.normalize(form.value)),
    })
  }

  return {
    form,
    isDirty: formLifecycle.isDirty,
    loading,
    reset: formLifecycle.reset,
    save,
    saving,
  }
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

function cloneSetting<TSetting extends object>(setting: TSetting): TSetting {
  return { ...setting }
}
