import { fireEvent } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../../test/render'
import AdminEditorActions from './AdminEditorActions.vue'

describe('AdminEditorActions', () => {
  it('renders reset before save and emits both actions', async () => {
    const { emitted } = render(AdminEditorActions, {
      props: { saveLabel: '保存', resetLabel: '重置' },
    })

    // 顺序即规范：重置永远在保存之前，改动此处即改全仓 editor 卡顺序
    const buttons = screen.getAllByRole('button')
    expect(buttons.map((button) => button.textContent)).toEqual(['重置', '保存'])

    await fireEvent.click(screen.getByRole('button', { name: '重置' }))
    await fireEvent.click(screen.getByRole('button', { name: '保存' }))

    expect(emitted().reset).toHaveLength(1)
    expect(emitted().save).toHaveLength(1)
  })

  it('disables save while saving and disables reset via resetDisabled', () => {
    render(AdminEditorActions, {
      props: { saveLabel: '保存', resetLabel: '重置', saving: true, resetDisabled: true },
    })

    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '重置' })).toBeDisabled()
  })

  it('falls back to the shared 保存/重置 defaults without label props', () => {
    render(AdminEditorActions, {
      props: {},
    })

    const buttons = screen.getAllByRole('button')
    expect(buttons.map((button) => button.textContent)).toEqual(['重置', '保存'])
  })

  it('renders the leading slot for hint or destructive actions', () => {
    render(AdminEditorActions, {
      props: { saveLabel: '保存' },
      slots: { leading: '<span>前置说明</span>' },
    })

    expect(screen.getByRole('button', { name: '保存' })).toBeVisible()
    expect(screen.getByText('前置说明')).toBeVisible()
  })
})
