import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminToolbarSearch from './AdminToolbarSearch.vue'

describe('AdminToolbarSearch', () => {
  it('uses fluid layout and preserves the accessible search label', () => {
    renderWithUser(AdminToolbarSearch, {
      props: {
        modelValue: '',
        inputLabel: '搜索模型',
        fluid: true,
      },
    })

    const input = screen.getByRole('textbox', { name: '搜索模型' })
    expect(input.parentElement).toHaveClass('min-w-[220px]', 'max-w-md', 'flex-1')
    expect(input).toHaveAttribute('aria-label', '搜索模型')
  })

  it('clears the value and emits clear when the clear button is used', async () => {
    const result = renderWithUser(AdminToolbarSearch, {
      props: {
        modelValue: 'claude',
        inputLabel: '搜索模型',
      },
    })

    await result.user.click(screen.getByRole('button', { name: '清空搜索' }))

    expect(result.emitted('update:modelValue')).toEqual([['']])
    expect(result.emitted('clear')).toEqual([[]])
  })
})
