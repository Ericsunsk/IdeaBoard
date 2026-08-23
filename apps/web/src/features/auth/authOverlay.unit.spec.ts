import { beforeEach, describe, expect, it } from 'vitest'
import { createAuthOverlay } from './authOverlay'

describe('authOverlay', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

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

  it('persists and consumes a safe post-auth destination exactly once', () => {
    const overlay = createAuthOverlay()

    overlay.commands.open('login', '/user/boards?tab=recent')

    expect(overlay.commands.consumePostAuthPath()).toBe('/user/boards?tab=recent')
    expect(overlay.commands.consumePostAuthPath()).toBeNull()
  })

  it('rejects unsafe post-auth destinations', () => {
    const overlay = createAuthOverlay()

    overlay.commands.open('login', 'https://example.com')

    expect(overlay.commands.consumePostAuthPath()).toBeNull()
  })
})
