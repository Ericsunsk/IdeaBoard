import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import AdminNumberInput from './AdminNumberInput.vue'

describe('AdminNumberInput', () => {
  it('keeps suffix out of the input while entering multiple digits', async () => {
    const result = renderWithUser(AdminNumberInput, {
      props: { suffix: ' Ideas' },
    })
    const input = screen.getByRole('spinbutton')

    await result.user.type(input, '123')

    expect(input).toHaveValue('123')
    expect(result.emitted('update:modelValue')).toEqual([[1], [12], [123]])
  })

  it('emits null when a suffixed value is cleared', async () => {
    const result = renderWithUser(AdminNumberInput, {
      props: { modelValue: 42, suffix: ' Ideas' },
    })
    const input = screen.getByRole('spinbutton')

    await result.user.clear(input)
    await result.rerender({ modelValue: null, suffix: ' Ideas' })

    expect(input).toHaveValue('')
    expect(result.emitted('update:modelValue')).toEqual([[null]])
  })

  it('still clamps ordinary numeric input to min and max', async () => {
    const result = renderWithUser(AdminNumberInput, {
      props: { min: 1, max: 10 },
    })
    const input = screen.getByRole('spinbutton')

    await result.user.type(input, '0')
    await result.user.clear(input)
    await result.user.type(input, '11')

    expect(result.emitted('update:modelValue')).toEqual([[1], [null], [1], [10]])
  })
})
