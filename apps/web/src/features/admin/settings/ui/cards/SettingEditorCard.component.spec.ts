import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import SettingEditorCard from './SettingEditorCard.vue'

describe('SettingEditorCard', () => {
  it('keeps the title, header-actions, editor body, and actions in one stable card contract', () => {
    renderWithUser(SettingEditorCard, {
      global: {
        plugins: [],
      },
      props: {
        title: '测试设置',
      },
      slots: {
        actions: '<button type="button">重置</button>',
        default: '<label><span>编辑值</span><input value="current" /></label>',
        'header-actions': '<span>已生效</span>',
      },
    })

    expect(screen.getByText('测试设置')).toBeVisible()
    expect(screen.getByText('已生效')).toBeVisible()
    expect(screen.getByLabelText('编辑值')).toHaveValue('current')
    expect(screen.getByRole('button', { name: '重置' })).toBeVisible()
  })
})
