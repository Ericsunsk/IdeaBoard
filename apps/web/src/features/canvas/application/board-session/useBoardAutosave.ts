import { tryOnScopeDispose, useDebounceFn, useDocumentVisibility, useOnline } from '@vueuse/core'
import { ref, watch, type Ref } from 'vue'
import { recordCanvasInteractionCount } from '../../infrastructure/browser/canvasInteractionMetrics.ts'

export type UseBoardAutosaveOptions = {
  isDirty: Ref<boolean>
  isSaving: Ref<boolean>
  save: (options?: { silent?: boolean | undefined }) => Promise<boolean>
  canRun?: (() => boolean) | undefined
  resumeSignal?: Ref<unknown> | undefined
  delay?: number | undefined
}

export function useBoardAutosave(options: UseBoardAutosaveOptions) {
  const delay = options.delay ?? 1200
  const visibility = useDocumentVisibility()
  const online = useOnline()
  const hasPendingSave = ref(false)
  const dirtyVersion = ref(0)
  let activeSaveTask: Promise<void> | null = null
  // A pending debounced trigger is not scope-bound: without this guard it
  // would fire after unmount, run a save against torn-down board state, and
  // surface a post-unmount error toast via settleSaveFailure. Board switches
  // and leave guards flush explicitly, so dropping the trailing debounce on
  // dispose loses nothing.
  let disposed = false
  // Intentional `tryOnScopeDispose`: the autosave coordinator stays safe when
  // instantiated outside a component scope; the trailing debounce drops on
  // dispose either way (see above).
  tryOnScopeDispose(() => {
    disposed = true
  })

  async function runSaveLoop(silent = true) {
    let nextSilent = silent
    do {
      hasPendingSave.value = false
      if (disposed) return
      if (!options.isDirty.value) return
      if (options.canRun && !options.canRun()) {
        hasPendingSave.value = true
        return
      }
      if (options.isSaving.value) {
        hasPendingSave.value = true
        return
      }
      const saveVersion = dirtyVersion.value
      recordCanvasInteractionCount('autosaveSaveCount')
      const saved = await options.save({ silent: nextSilent })
      if (disposed) return
      if (!saved) {
        hasPendingSave.value = true
        return
      }
      if (dirtyVersion.value > saveVersion) {
        options.isDirty.value = true
        hasPendingSave.value = true
      }
      nextSilent = true
    } while (hasPendingSave.value)
  }

  async function runSaveNow(silent = true) {
    if (disposed) return
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
    recordCanvasInteractionCount('autosaveScheduleCount')
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

  if (options.resumeSignal) {
    watch(
      options.resumeSignal,
      (next, prev) => {
        if (next === prev || !next) return
        if (options.isDirty.value || hasPendingSave.value) {
          void flushAutosave()
        }
      },
      { flush: 'sync' },
    )
  }

  return {
    markDirtyAndSchedule,
    flushAutosave,
  }
}
