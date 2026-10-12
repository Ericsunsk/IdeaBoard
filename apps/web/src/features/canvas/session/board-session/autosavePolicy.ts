/**
 * Board autosave policy — framework-free save-loop state machine.
 *
 * Owns debounce scheduling, dirty-version tracking, gated/deferred saves,
 * and explicit disposal. No Vue imports: reactivity enters through plain
 * accessor callbacks, and component lifecycle (visibility/online/resume
 * watchers, scope disposal) stays in the `useBoardAutosave` adapter.
 */

export type BoardAutosavePolicyDeps = {
  isDirty: () => boolean
  isSaving: () => boolean
  markDirty: () => void
  canRun?: (() => boolean) | undefined
  save: (options?: { isSilent?: boolean | undefined }) => Promise<boolean>
  delay?: number | undefined
  onSchedule?: (() => void) | undefined
  onSave?: (() => void) | undefined
}

export type BoardAutosavePolicy = {
  markDirtyAndSchedule: () => void
  flushAutosave: (options?: { isSilent?: boolean | undefined }) => Promise<void>
  handleHidden: () => void
  handleOnlineChange: (prev: boolean | undefined, next: boolean) => void
  handleResumeSignal: (next: unknown, prev: unknown) => void
  dispose: () => void
}

export function createBoardAutosavePolicy(deps: BoardAutosavePolicyDeps): BoardAutosavePolicy {
  const delay = deps.delay ?? 1200
  let hasPendingSave = false
  let dirtyVersion = 0
  let activeSaveTask: Promise<void> | null = null
  // A pending debounced trigger is not scope-bound: without this guard it
  // would fire after unmount, run a save against torn-down board state, and
  // surface a post-unmount error toast via settleSaveFailure. Board switches
  // and leave guards flush explicitly, so dropping the trailing debounce on
  // dispose loses nothing.
  let isDisposed = false
  let debounceTimer: ReturnType<typeof setTimeout> | undefined

  async function runSaveLoop(isSilent = true) {
    let isNextSilent = isSilent
    do {
      hasPendingSave = false
      if (isDisposed) return
      if (!deps.isDirty()) return
      if (deps.canRun && !deps.canRun()) {
        hasPendingSave = true
        return
      }
      if (deps.isSaving()) {
        hasPendingSave = true
        return
      }
      const saveVersion = dirtyVersion
      deps.onSave?.()
      const isSaved = await deps.save({ isSilent: isNextSilent })
      if (isDisposed) return
      if (!isSaved) {
        hasPendingSave = true
        return
      }
      if (dirtyVersion > saveVersion) {
        deps.markDirty()
        hasPendingSave = true
      }
      isNextSilent = true
    } while (hasPendingSave)
  }

  async function runSaveNow(isSilent = true) {
    if (isDisposed) return
    if (!deps.isDirty() && !hasPendingSave) return
    if (activeSaveTask) {
      hasPendingSave = true
      await activeSaveTask
      await runSaveNow(isSilent)
      return
    }
    activeSaveTask = runSaveLoop(isSilent).finally(() => {
      activeSaveTask = null
    })
    await activeSaveTask
  }

  function scheduleDebounced() {
    if (debounceTimer !== undefined) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      debounceTimer = undefined
      void runSaveNow(true)
    }, delay)
  }

  function markDirtyAndSchedule() {
    dirtyVersion += 1
    deps.onSchedule?.()
    scheduleDebounced()
  }

  async function flushAutosave(options?: { isSilent?: boolean | undefined }) {
    await runSaveNow(options?.isSilent ?? true)
  }

  function handleHidden() {
    void flushAutosave()
  }

  function handleOnlineChange(wasPreviouslyEnabled: boolean | undefined, isNextEnabled: boolean) {
    if (
      wasPreviouslyEnabled === false &&
      isNextEnabled === true &&
      (deps.isDirty() || hasPendingSave)
    ) {
      void flushAutosave()
    }
  }

  function handleResumeSignal(next: unknown, prev: unknown) {
    if (next === prev || !next) return
    if (deps.isDirty() || hasPendingSave) {
      void flushAutosave()
    }
  }

  function dispose() {
    isDisposed = true
    if (debounceTimer !== undefined) {
      clearTimeout(debounceTimer)
      debounceTimer = undefined
    }
  }

  return {
    markDirtyAndSchedule,
    flushAutosave,
    handleHidden,
    handleOnlineChange,
    handleResumeSignal,
    dispose,
  }
}
