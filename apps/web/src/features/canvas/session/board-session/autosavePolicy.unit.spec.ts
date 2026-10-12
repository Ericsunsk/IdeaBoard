import { describe, expect, it, vi } from 'vitest'
import { createBoardAutosavePolicy, type BoardAutosavePolicyDeps } from './autosavePolicy'

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

function createHarness(overrides: Partial<BoardAutosavePolicyDeps> = {}) {
  const state = { isDirty: false, isSaving: false }
  const defaultSave = vi.fn(async () => true)
  const policy = createBoardAutosavePolicy({
    isDirty: () => state.isDirty,
    isSaving: () => state.isSaving,
    markDirty: () => {
      state.isDirty = true
    },
    save: defaultSave,
    delay: 20,
    ...overrides,
  })
  const save = (overrides.save ?? defaultSave) as ReturnType<typeof vi.fn>
  return { state, save, policy }
}

describe('createBoardAutosavePolicy', () => {
  it('defers saves while gated and flushes when it can run', async () => {
    vi.useFakeTimers()
    try {
      let canRun = false
      const { state, save, policy } = createHarness({ canRun: () => canRun })

      state.isDirty = true
      policy.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).not.toHaveBeenCalled()
      expect(state.isDirty).toBe(true)

      canRun = true
      await policy.flushAutosave()

      expect(save).toHaveBeenCalledTimes(1)
      policy.dispose()
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps a failed save pending for explicit retry', async () => {
    vi.useFakeTimers()
    try {
      const { state, save, policy } = createHarness({
        save: vi.fn(async () => false),
      })

      state.isDirty = true
      policy.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).toHaveBeenCalledOnce()
      expect(state.isDirty).toBe(true)

      await policy.flushAutosave({ isSilent: false })

      expect(save).toHaveBeenCalledTimes(2)
      expect(save).toHaveBeenLastCalledWith({ isSilent: false })
      policy.dispose()
    } finally {
      vi.useRealTimers()
    }
  })

  it('retries a pending failed save when the resume signal changes', async () => {
    vi.useFakeTimers()
    try {
      const { state, save, policy } = createHarness({
        save: vi
          .fn<(options?: { isSilent?: boolean | undefined }) => Promise<boolean>>()
          .mockResolvedValueOnce(false)
          .mockImplementation(async () => {
            state.isDirty = false
            return true
          }),
      })

      state.isDirty = true
      policy.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).toHaveBeenCalledTimes(1)
      expect(state.isDirty).toBe(true)

      policy.handleResumeSignal('1:user-a', '')
      await flushPromises()

      expect(save).toHaveBeenCalledTimes(2)
      expect(state.isDirty).toBe(false)
      policy.dispose()
    } finally {
      vi.useRealTimers()
    }
  })

  it('flushes on hidden and on offline-to-online transitions', async () => {
    vi.useFakeTimers()
    try {
      const { state, save, policy } = createHarness()

      state.isDirty = true
      policy.handleHidden()
      await flushPromises()
      expect(save).toHaveBeenCalledTimes(1)

      state.isDirty = true
      policy.handleOnlineChange(false, true)
      await flushPromises()
      expect(save).toHaveBeenCalledTimes(2)

      policy.handleOnlineChange(true, true)
      await flushPromises()
      expect(save).toHaveBeenCalledTimes(2)
      policy.dispose()
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops a pending debounced save on dispose', async () => {
    vi.useFakeTimers()
    try {
      const { state, save, policy } = createHarness()
      state.isDirty = true

      policy.markDirtyAndSchedule()
      policy.dispose()
      await vi.advanceTimersByTimeAsync(50)
      await flushPromises()

      expect(save).not.toHaveBeenCalled()

      await policy.flushAutosave()
      expect(save).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('marks dirty when the draft advanced during a save', async () => {
    vi.useFakeTimers()
    try {
      const { state, save, policy } = createHarness()
      let isAdvanced = false
      save.mockImplementation(async () => {
        state.isDirty = false
        if (!isAdvanced) {
          isAdvanced = true
          policy.markDirtyAndSchedule()
        }
        return true
      })

      state.isDirty = true
      await policy.flushAutosave()
      expect(save).toHaveBeenCalledTimes(2)
      expect(state.isDirty).toBe(false)

      // The trailing debounce from the mid-save edit finds no dirty
      // state and issues no extra save.
      await vi.advanceTimersByTimeAsync(50)
      await flushPromises()
      expect(save).toHaveBeenCalledTimes(2)
      policy.dispose()
    } finally {
      vi.useRealTimers()
    }
  })
})
