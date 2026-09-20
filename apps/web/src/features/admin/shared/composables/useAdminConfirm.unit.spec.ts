import { describe, expect, it, vi } from 'vitest'
import { activeAdminConfirm, useAdminConfirm } from './useAdminConfirm'

describe('useAdminConfirm', () => {
  it('updates reactive confirm state and executes onConfirm when approved', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const adminConfirm = useAdminConfirm()

    expect(activeAdminConfirm.value).toBeNull()

    adminConfirm.request({
      title: '确认删除',
      message: '确定删除吗？',
      confirmLabel: '确定',
      intent: 'danger',
      onConfirm,
    })

    expect(activeAdminConfirm.value).not.toBeNull()
    expect(activeAdminConfirm.value?.title).toBe('确认删除')
    expect(activeAdminConfirm.value?.message).toBe('确定删除吗？')
    expect(activeAdminConfirm.value?.confirmLabel).toBe('确定')
    expect(activeAdminConfirm.value?.intent).toBe('danger')
    expect(activeAdminConfirm.value?.open).toBe(true)

    await activeAdminConfirm.value?.confirm()

    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(activeAdminConfirm.value).toBeNull()
  })

  it('clears reactive confirm state when cancelled', () => {
    const onConfirm = vi.fn()
    const adminConfirm = useAdminConfirm()

    adminConfirm.request({
      title: '确认重置',
      message: '确定重置吗？',
      confirmLabel: '重置',
      intent: 'warn',
      onConfirm,
    })

    expect(activeAdminConfirm.value?.open).toBe(true)
    activeAdminConfirm.value?.cancel()

    expect(onConfirm).not.toHaveBeenCalled()
    expect(activeAdminConfirm.value).toBeNull()
  })
})
