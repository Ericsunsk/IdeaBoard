import { tryOnScopeDispose, useDocumentVisibility, useOnline } from '@vueuse/core'
import { watch, type Ref } from 'vue'
import { recordCanvasInteractionCount } from '../browser/canvasInteractionMetrics.ts'
import { createBoardAutosavePolicy } from './autosavePolicy'

export type UseBoardAutosaveOptions = {
  isDirty: Ref<boolean>
  isSaving: Ref<boolean>
  save: (options?: { silent?: boolean | undefined }) => Promise<boolean>
  canRun?: (() => boolean) | undefined
  resumeSignal?: Ref<unknown> | undefined
  delay?: number | undefined
}

/**
 * Vue lifecycle adapter over the framework-free autosave policy.
 * Owns document-visibility/online/resume watchers and scope disposal;
 * the save-loop decision logic lives in `autosavePolicy`.
 */
export function useBoardAutosave(options: UseBoardAutosaveOptions) {
  const policy = createBoardAutosavePolicy({
    isDirty: () => options.isDirty.value,
    isSaving: () => options.isSaving.value,
    markDirty: () => {
      options.isDirty.value = true
    },
    canRun: options.canRun,
    save: options.save,
    delay: options.delay,
    onSchedule: () => recordCanvasInteractionCount('autosaveScheduleCount'),
    onSave: () => recordCanvasInteractionCount('autosaveSaveCount'),
  })
  const visibility = useDocumentVisibility()
  const online = useOnline()
  // Intentional `tryOnScopeDispose`: the autosave coordinator stays safe when
  // instantiated outside a component scope; the trailing debounce drops on
  // dispose either way (see the policy).
  tryOnScopeDispose(() => {
    policy.dispose()
  })

  watch(visibility, (state) => {
    if (state === 'hidden') {
      policy.handleHidden()
    }
  })

  watch(online, (next, prev) => {
    policy.handleOnlineChange(prev, next)
  })

  if (options.resumeSignal) {
    watch(
      options.resumeSignal,
      (next, prev) => {
        policy.handleResumeSignal(next, prev)
      },
      { flush: 'sync' },
    )
  }

  return {
    markDirtyAndSchedule: policy.markDirtyAndSchedule,
    flushAutosave: policy.flushAutosave,
  }
}
