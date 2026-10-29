import { describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { activeConfirm, useConfirm, useConfirmDialog } from './useConfirmDialog'

describe('useConfirmDialog', () => {
  it('handles traditional callback request and confirmation', async () => {
    const dialog = useConfirmDialog()
    const action = vi.fn().mockResolvedValue(undefined)

    dialog.request({ title: '删除文件', message: '确定删除吗？' }, action)

    expect(dialog.visible.value).toBe(true)
    expect(dialog.title.value).toBe('删除文件')

    await dialog.confirm()

    expect(action).toHaveBeenCalledTimes(1)
    expect(dialog.visible.value).toBe(false)
  })

  it('handles confirmAsync Promise returning true when confirmed', async () => {
    const dialog = useConfirmDialog()
    const promise = dialog.confirmAsync({
      title: '异步确认',
      message: '确定继续？',
      intent: 'danger',
    })

    expect(dialog.visible.value).toBe(true)
    expect(dialog.intent.value).toBe('danger')

    await dialog.confirm()

    const result = await promise
    expect(result).toBe(true)
    expect(dialog.visible.value).toBe(false)
  })

  it('handles confirmAsync Promise returning false when cancelled', async () => {
    const dialog = useConfirmDialog()
    const promise = dialog.confirmAsync({
      title: '取消测试',
      message: '测试取消',
    })

    expect(dialog.visible.value).toBe(true)
    dialog.close()

    const result = await promise
    expect(result).toBe(false)
    expect(dialog.visible.value).toBe(false)
  })

  it('resolves a pending confirmAsync with false on dispose so unmount cannot orphan it', async () => {
    const dialog = useConfirmDialog()
    const promise = dialog.confirmAsync({
      title: '卸载测试',
      message: '父组件卸载',
    })

    expect(dialog.visible.value).toBe(true)
    dialog.dispose()

    await expect(promise).resolves.toBe(false)
    expect(dialog.visible.value).toBe(false)
  })

  it('resolves a pending confirmAsync with false when the scope stops', async () => {
    const scope = effectScope()
    let promise!: Promise<boolean>
    scope.run(() => {
      const dialog = useConfirmDialog()
      promise = dialog.confirmAsync({
        title: '作用域测试',
        message: '作用域停止',
      })
    })

    scope.stop()

    await expect(promise).resolves.toBe(false)
  })
})

describe('useConfirm', () => {
  it('updates reactive confirm state and resolves true when approved', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const confirm = useConfirm()

    expect(activeConfirm.value).toBeNull()

    const promise = confirm.confirm({
      title: '确认删除',
      message: '确定删除吗？',
      confirmLabel: '确定',
      intent: 'danger',
      onConfirm,
    })

    expect(activeConfirm.value).not.toBeNull()
    expect(activeConfirm.value?.title).toBe('确认删除')
    expect(activeConfirm.value?.message).toBe('确定删除吗？')
    expect(activeConfirm.value?.confirmLabel).toBe('确定')
    expect(activeConfirm.value?.intent).toBe('danger')
    expect(activeConfirm.value?.open).toBe(true)

    await activeConfirm.value?.confirm()

    expect(onConfirm).toHaveBeenCalledTimes(1)
    await expect(promise).resolves.toBe(true)
    expect(activeConfirm.value).toBeNull()
  })

  it('clears reactive confirm state and resolves false when cancelled', async () => {
    const onConfirm = vi.fn()
    const confirm = useConfirm()

    const promise = confirm.confirm({
      title: '确认重置',
      message: '确定重置吗？',
      confirmLabel: '重置',
      intent: 'warn',
      onConfirm,
    })

    expect(activeConfirm.value?.open).toBe(true)
    activeConfirm.value?.cancel()

    await expect(promise).resolves.toBe(false)
    expect(onConfirm).not.toHaveBeenCalled()
    expect(activeConfirm.value).toBeNull()
  })

  it('supports Promise-based confirm() workflow returning boolean', async () => {
    const confirm = useConfirm()
    const promise = confirm.confirm({
      title: 'Promise 确认',
      message: '是否同意？',
      confirmLabel: '同意',
      intent: 'danger',
    })

    expect(activeConfirm.value?.open).toBe(true)
    await activeConfirm.value?.confirm()

    const result = await promise
    expect(result).toBe(true)
    expect(activeConfirm.value).toBeNull()
  })

  it('rejects the promise when onConfirm throws', async () => {
    const confirm = useConfirm()
    const error = new Error('operation failed')
    const promise = confirm.confirm({
      title: '错误测试',
      message: '测试',
      confirmLabel: '确认',
      intent: 'danger',
      onConfirm: async () => {
        throw error
      },
    })

    await expect(activeConfirm.value?.confirm()).rejects.toThrow('operation failed')
    await expect(promise).rejects.toThrow('operation failed')
  })
})
