import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowAutosave } from './useFlowAutosave'

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('useFlowAutosave', () => {
  it('defers saves while autosave is gated and flushes when it can run', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(false)
      const isSaving = ref(false)
      const canRun = ref(false)
      const save = vi.fn(async () => true)
      const autosave = useFlowAutosave({
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
})
