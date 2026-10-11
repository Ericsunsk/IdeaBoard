import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminActionReasonDialog from './AdminActionReasonDialog.vue'

function renderDialog(visible = true) {
  const onVisibleUpdate = vi.fn()
  return {
    onVisibleUpdate,
    ...renderWithUser(AdminActionReasonDialog, {
      props: {
        visible,
        title: '确认重跑检查',
        message: '本次操作会写入审计记录。',
        confirmLabel: '确认重跑',
        'onUpdate:visible': onVisibleUpdate,
      },
      global: {
        stubs: { transition: false },
      },
    }),
  }
}

describe('AdminActionReasonDialog', () => {
  it('clears transient input and validation state before every reopen', async () => {
    const result = renderDialog()
    const reasonInput = await screen.findByLabelText(/操作原因/)

    await result.user.type(reasonInput, '第一次填写')
    await result.user.clear(reasonInput)
    await result.user.click(screen.getByRole('button', { name: '确认重跑' }))
    expect(screen.getByText('请输入操作原因')).toBeInTheDocument()

    await result.rerender({
      visible: false,
      title: '确认重跑检查',
      message: '本次操作会写入审计记录。',
      confirmLabel: '确认重跑',
    })
    await result.rerender({
      visible: true,
      title: '确认重跑检查',
      message: '本次操作会写入审计记录。',
      confirmLabel: '确认重跑',
    })

    expect(await screen.findByLabelText(/操作原因/)).toHaveValue('')
    expect(screen.queryByText('请输入操作原因')).not.toBeInTheDocument()
  })

  it('routes the footer close control through the shared lifecycle', async () => {
    const result = renderDialog()

    await screen.findByRole('dialog', { name: '确认重跑检查' })
    await result.user.click(screen.getByRole('button', { name: '取消' }))

    expect(result.onVisibleUpdate).toHaveBeenCalledWith(false)
  })
})
