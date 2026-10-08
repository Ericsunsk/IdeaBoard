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
})
