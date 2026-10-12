import { describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useBoardAutosave } from './useBoardAutosave'

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('useBoardAutosave', () => {
  it('defers saves while autosave is gated and flushes when it can run', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(false)
      const isSaving = ref(false)
      const canRun = ref(false)
      const save = vi.fn(async () => true)
      const autosave = useBoardAutosave({
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
        .fn<(options?: { isSilent?: boolean | undefined }) => Promise<boolean>>()
        .mockResolvedValueOnce(false)
        .mockImplementation(async () => {
          isDirty.value = false
          return true
        })
      const autosave = useBoardAutosave({
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

  it('keeps a failed save pending for explicit retry', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(false)
      const isSaving = ref(false)
      const save = vi.fn(async () => false)
      const autosave = useBoardAutosave({
        isDirty,
        isSaving,
        save,
        delay: 20,
      })

      isDirty.value = true
      autosave.markDirtyAndSchedule()
      await vi.advanceTimersByTimeAsync(20)
      await flushPromises()

      expect(save).toHaveBeenCalledOnce()
      expect(isDirty.value).toBe(true)

      await autosave.flushAutosave({ isSilent: false })

      expect(save).toHaveBeenCalledTimes(2)
      expect(save).toHaveBeenLastCalledWith({ isSilent: false })
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops a pending debounced save when the scope is disposed', async () => {
    vi.useFakeTimers()
    try {
      const isDirty = ref(true)
      const isSaving = ref(false)
      const save = vi.fn(async () => true)
      const scope = effectScope()
      const autosave = scope.run(() => useBoardAutosave({ isDirty, isSaving, save, delay: 20 }))
      if (!autosave) throw new Error('autosave was not created')

      autosave.markDirtyAndSchedule()
      scope.stop()
      await vi.advanceTimersByTimeAsync(50)
      await flushPromises()

      expect(save).not.toHaveBeenCalled()

      await autosave.flushAutosave()
      expect(save).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })
})
