import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import SettingEditorCard from './SettingEditorCard.vue'

describe('SettingEditorCard', () => {
  it('keeps the title, status, editor body, and actions in one stable card contract', () => {
    renderWithUser(SettingEditorCard, {
      global: {
        plugins: [],
      },
      props: {
        status: '已生效',
        statusSeverity: 'success',
        title: '测试设置',
      },
      slots: {
        actions: '<button type="button">重置</button>',
        default: '<label><span>编辑值</span><input value="current" /></label>',
      },
    })

    expect(screen.getByText('测试设置')).toBeVisible()
    expect(screen.getByText('已生效')).toBeVisible()
    expect(screen.getByLabelText('编辑值')).toHaveValue('current')
    expect(screen.getByRole('button', { name: '重置' })).toBeVisible()
  })
})
