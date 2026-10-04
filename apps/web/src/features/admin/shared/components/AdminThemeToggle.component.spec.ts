import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import AdminThemeToggle from './AdminThemeToggle.vue'

describe('AdminThemeToggle', () => {
  it('renders theme trigger button and opens options on click', async () => {
    const { user } = renderWithUser(h(AdminThemeToggle))

    const button = screen.getByRole('button', { name: '切换主题外观' })
    expect(button).toBeInTheDocument()

    await user.click(button)

    expect(await screen.findByRole('menuitem', { name: /明亮/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /暗黑/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /跟随系统/i })).toBeInTheDocument()
  })
})
