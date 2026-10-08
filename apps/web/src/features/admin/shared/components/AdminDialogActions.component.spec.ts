import { fireEvent } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import AdminDialogActions from './AdminDialogActions.vue'

describe('AdminDialogActions', () => {
  it('renders cancel and confirm labels and emits both actions', async () => {
    const { emitted } = render(AdminDialogActions, {
      props: { cancelLabel: '取消', confirmLabel: '保存' },
    })

    await fireEvent.click(screen.getByRole('button', { name: '取消' }))
    await fireEvent.click(screen.getByRole('button', { name: '保存' }))

    expect(emitted().cancel).toHaveLength(1)
    expect(emitted().confirm).toHaveLength(1)
  })

  it('disables confirm while loading and disables cancel via cancelDisabled', () => {
    render(AdminDialogActions, {
      props: { cancelLabel: '取消', confirmLabel: '保存', loading: true, cancelDisabled: true },
    })

    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '取消' })).toBeDisabled()
  })

  it('supports an extra confirm disabled state without loading', () => {
    render(AdminDialogActions, {
      props: { cancelLabel: '取消', confirmLabel: '保存', disabled: true },
    })

    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '取消' })).not.toBeDisabled()
  })

  it('falls back to the shared 取消/确认 defaults without label props', () => {
    render(AdminDialogActions, {
      props: {},
    })

    const buttons = screen.getAllByRole('button')
    expect(buttons.map((button) => button.textContent)).toEqual(['取消', '确认'])
  })

  it('renders extra actions between cancel and confirm without hijacking emits', async () => {
    const { emitted } = render(AdminDialogActions, {
      props: { cancelLabel: '取消', confirmLabel: '保存' },
      slots: { 'extra-actions': '<button type="button">测试连接</button>' },
    })

    const buttons = screen.getAllByRole('button')
    expect(buttons.map((button) => button.textContent)).toEqual(['取消', '测试连接', '保存'])

    await fireEvent.click(screen.getByRole('button', { name: '测试连接' }))
    expect(emitted().cancel).toBeUndefined()
    expect(emitted().confirm).toBeUndefined()
  })

  it('renders only cancel and confirm when no extra actions are provided', () => {
    render(AdminDialogActions, {
      props: { cancelLabel: '取消', confirmLabel: '保存' },
    })

    expect(screen.getAllByRole('button')).toHaveLength(2)
  })
})
