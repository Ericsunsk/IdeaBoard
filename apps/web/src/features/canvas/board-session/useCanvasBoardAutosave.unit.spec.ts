import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useCanvasBoardAutosave } from './useCanvasBoardAutosave'

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('useCanvasBoardAutosave', () => {
  it('defers saves while autosave is gated and flushes when it can run', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(false)
      const isSaving = ref(false)
      const canRun = ref(false)
      const save = vi.fn(async () => true)
      const autosave = useCanvasBoardAutosave({
        isDirty,
        isSaving,
        canRun: () => canRun.value,
        save,
        delay: 20,
      })

      isDirty.value = true
      autosave.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).not.toHaveBeenCalled()
      expect(isDirty.value).toBe(true)

      canRun.value = true
      await autosave.flushAutosave()

      expect(save).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('retries a pending failed save when the resume signal changes', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(false)
      const isSaving = ref(false)
      const resumeSignal = ref('')
      const save = vi
        .fn<(options?: { silent?: boolean }) => Promise<boolean>>()
        .mockResolvedValueOnce(false)
        .mockImplementation(async () => {
          isDirty.value = false
          return true
        })
      const autosave = useCanvasBoardAutosave({
        isDirty,
        isSaving,
        resumeSignal,
        save,
        delay: 20,
      })

      isDirty.value = true
      autosave.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).toHaveBeenCalledTimes(1)
      expect(isDirty.value).toBe(true)

      resumeSignal.value = '1:user-a'
      await flushPromises()

      expect(save).toHaveBeenCalledTimes(2)
      expect(isDirty.value).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })
})
