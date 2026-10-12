import { describe, expect, it, vi } from 'vitest'
import { activeAdminConfirm, useAdminConfirm } from './useAdminConfirm'

describe('useAdminConfirm', () => {
  it('updates reactive confirm state and resolves true when approved', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const adminConfirm = useAdminConfirm()

    expect(activeAdminConfirm.value).toBeNull()

    const promise = adminConfirm.confirm({
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
    await expect(promise).resolves.toBe(true)
    expect(activeAdminConfirm.value).toBeNull()
  })

  it('clears reactive confirm state and resolves false when cancelled', async () => {
    const onConfirm = vi.fn()
    const adminConfirm = useAdminConfirm()

    const promise = adminConfirm.confirm({
      title: '确认重置',
      message: '确定重置吗？',
      confirmLabel: '重置',
      intent: 'warn',
      onConfirm,
    })

    expect(activeAdminConfirm.value?.open).toBe(true)
    activeAdminConfirm.value?.cancel()

    await expect(promise).resolves.toBe(false)
    expect(onConfirm).not.toHaveBeenCalled()
    expect(activeAdminConfirm.value).toBeNull()
  })

  it('supports Promise-based confirm() workflow returning boolean', async () => {
    const adminConfirm = useAdminConfirm()
    const promise = adminConfirm.confirm({
      title: 'Promise 确认',
      message: '是否同意？',
      confirmLabel: '同意',
      intent: 'danger',
    })

    expect(activeAdminConfirm.value?.open).toBe(true)
    await activeAdminConfirm.value?.confirm()

    const result = await promise
    expect(result).toBe(true)
    expect(activeAdminConfirm.value).toBeNull()
  })

  it('rejects the promise when onConfirm throws', async () => {
    const adminConfirm = useAdminConfirm()
    const error = new Error('operation failed')
    const promise = adminConfirm.confirm({
      title: '错误测试',
      message: '测试',
      confirmLabel: '确认',
      intent: 'danger',
      onConfirm: async () => {
        throw error
      },
    })

    await expect(activeAdminConfirm.value?.confirm()).rejects.toThrow('operation failed')
    await expect(promise).rejects.toThrow('operation failed')
  })

  it('confirms with an empty note when the reason is optional', async () => {
    const adminConfirm = useAdminConfirm()
    const promise = adminConfirm.confirm({
      title: '启用功能',
      message: '确定启用吗？',
      confirmLabel: '启用',
      intent: 'warn',
      reason: { isRequired: false },
    })

    expect(activeAdminConfirm.value?.reasonEnabled).toBe(true)
    expect(activeAdminConfirm.value?.reasonRequired).toBe(false)
    await activeAdminConfirm.value?.confirm()

    await expect(promise).resolves.toBe(true)
    expect(activeAdminConfirm.value).toBeNull()
  })

  it('blocks confirmation on an empty required note without resolving', async () => {
    const adminConfirm = useAdminConfirm()
    let resolved = false
    const promise = adminConfirm
      .confirm({
        title: '删除功能',
        message: '确定删除吗？',
        confirmLabel: '删除',
        intent: 'danger',
        reason: { isRequired: true },
      })
      .then((value) => {
        resolved = true
        return value
      })

    await activeAdminConfirm.value?.confirm()
    await Promise.resolve()

    expect(resolved).toBe(false)
    expect(activeAdminConfirm.value?.open).toBe(true)
    activeAdminConfirm.value?.setReason('audit note')
    await activeAdminConfirm.value?.confirm()
    await expect(promise).resolves.toBe(true)
  })
})
