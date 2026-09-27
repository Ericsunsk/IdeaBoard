import { describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { useUserConfirmDialog } from './useUserConfirmDialog'

describe('useUserConfirmDialog', () => {
  it('handles traditional callback request and confirmation', async () => {
    const dialog = useUserConfirmDialog()
    const action = vi.fn().mockResolvedValue(undefined)

    dialog.request({ title: '删除文件', message: '确定删除吗？' }, action)

    expect(dialog.visible.value).toBe(true)
    expect(dialog.title.value).toBe('删除文件')

    await dialog.confirm()

    expect(action).toHaveBeenCalledTimes(1)
    expect(dialog.visible.value).toBe(false)
  })

  it('handles confirmAsync Promise returning true when confirmed', async () => {
    const dialog = useUserConfirmDialog()
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
    const dialog = useUserConfirmDialog()
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
    const dialog = useUserConfirmDialog()
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
      const dialog = useUserConfirmDialog()
      promise = dialog.confirmAsync({
        title: '作用域测试',
        message: '作用域停止',
      })
    })

    scope.stop()

    await expect(promise).resolves.toBe(false)
  })
})
