import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import type { RetouchSharpenParams } from '../types'
import RetouchSharpenControl from './RetouchSharpenControl.vue'

function setInputValue(target: HTMLElement, value: string) {
  const input = target as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('RetouchSharpenControl browser smoke', () => {
  it('updates sharpening controls through the rendered browser UI', async () => {
    const updateParams = vi.fn()
    const params: RetouchSharpenParams = {
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    }

    const { user } = renderWithUser(RetouchSharpenControl, {
      props: { params, 'onUpdate:params': updateParams },
    })

    setInputValue(screen.getByLabelText('Amount slider'), '120')
    await waitFor(() => expect(updateParams).toHaveBeenCalledWith({ ...params, amount: 120 }))

    const protectColorButton = screen.getByRole('button', { name: 'Protect sharpen colors' })
    await user.click(protectColorButton)
    expect(updateParams).toHaveBeenLastCalledWith({ ...params, protectColor: false })
  })
})
