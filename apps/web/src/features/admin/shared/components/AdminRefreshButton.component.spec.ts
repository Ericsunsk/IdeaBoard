import { fireEvent } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../test/render'
import AdminRefreshButton from './AdminRefreshButton.vue'

describe('AdminRefreshButton', () => {
  it('renders loading state and emits clicks when enabled', async () => {
    const onClick = vi.fn()
    const { rerender } = render(AdminRefreshButton, {
      props: { ariaLabel: '刷新模型', onClick },
    })

    const button = screen.getByRole('button', { name: '刷新模型' })
    expect(button).toHaveTextContent('刷新')
    await fireEvent.click(button)
    expect(onClick).toHaveBeenCalledOnce()

    await rerender({ ariaLabel: '刷新模型', loading: true, onClick })
    expect(button).toBeDisabled()
    expect(button).toHaveTextContent('同步中...')
  })
})
