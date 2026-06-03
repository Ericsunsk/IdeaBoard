import { useDebounceFn, useDocumentVisibility, useOnline } from '@vueuse/core'
import { ref, watch, type Ref } from 'vue'

type UseFlowAutosaveOptions = {
  isDirty: Ref<boolean>
  isSaving: Ref<boolean>
  save: (options?: { silent?: boolean }) => Promise<boolean>
  delay?: number
}

export function useFlowAutosave(options: UseFlowAutosaveOptions) {
  const delay = options.delay ?? 1200
  const visibility = useDocumentVisibility()
  const online = useOnline()
  const hasPendingSave = ref(false)
  const dirtyVersion = ref(0)
  let activeSaveTask: Promise<void> | null = null

  async function runSaveLoop(silent = true) {
    let nextSilent = silent
    do {
      hasPendingSave.value = false
      if (!options.isDirty.value) return
      if (options.isSaving.value) {
        hasPendingSave.value = true
        return
      }
      const saveVersion = dirtyVersion.value
      const saved = await options.save({ silent: nextSilent })
      if (!saved) return
      if (dirtyVersion.value > saveVersion) {
        options.isDirty.value = true
        hasPendingSave.value = true
      }
      nextSilent = true
    } while (hasPendingSave.value)
  }

  async function runSaveNow(silent = true) {
    if (!options.isDirty.value && !hasPendingSave.value) return
    if (activeSaveTask) {
      hasPendingSave.value = true
      await activeSaveTask
      await runSaveNow(silent)
      return
    }
    activeSaveTask = runSaveLoop(silent).finally(() => {
      activeSaveTask = null
    })
    await activeSaveTask
  }

  const triggerAutosave = useDebounceFn(() => {
    void runSaveNow(true)
  }, delay)

  function markDirtyAndSchedule() {
    dirtyVersion.value += 1
    triggerAutosave()
  }

  async function flushAutosave(options?: { silent?: boolean }) {
    await runSaveNow(options?.silent ?? true)
  }

  watch(visibility, (state) => {
    if (state === 'hidden') {
      void flushAutosave()
    }
  })

  watch(online, (next, prev) => {
    if (prev === false && next === true && (options.isDirty.value || hasPendingSave.value)) {
      void flushAutosave()
    }
  })

  return {
    markDirtyAndSchedule,
    flushAutosave,
  }
}
