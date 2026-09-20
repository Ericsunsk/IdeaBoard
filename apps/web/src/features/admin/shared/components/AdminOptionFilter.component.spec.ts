import { fireEvent } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen } from '../../../../test/render'
import AdminOptionFilter from './AdminOptionFilter.vue'

describe('AdminOptionFilter', () => {
  it('exposes listbox semantics and closes after selecting an option', async () => {
    const value = ref('all')
    render(AdminOptionFilter, {
      props: {
        modelValue: value.value,
        options: [
          { label: '全部状态', value: 'all' },
          { label: '已配置', value: 'configured' },
        ],
        allValue: 'all',
        label: '筛选提示词状态',
        'onUpdate:modelValue': (nextValue: string) => {
          value.value = nextValue
        },
      },
    })

    const trigger = screen.getByRole('button', { name: '筛选提示词状态' })
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
    expect(trigger).toHaveAttribute('aria-expanded', 'false')

    await fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await fireEvent.click(screen.getByRole('option', { name: '已配置' }))
    expect(value.value).toBe('configured')
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })
})
