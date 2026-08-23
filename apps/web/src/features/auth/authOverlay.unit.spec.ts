import { describe, expect, it } from 'vitest'
import { createAuthOverlay } from './authOverlay'

describe('authOverlay', () => {
  it('keeps authentication presentation state independent from the current route', () => {
    const overlay = createAuthOverlay()

    expect(overlay.viewModel.visible.value).toBe(false)
    overlay.commands.open('login')
    expect(overlay.viewModel.mode.value).toBe('login')

    overlay.commands.open('register')
    expect(overlay.viewModel.mode.value).toBe('register')

    overlay.commands.close()
    expect(overlay.viewModel.visible.value).toBe(false)
  })
})
