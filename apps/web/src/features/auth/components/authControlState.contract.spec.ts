import { describe, expect, it } from 'vitest'
import { authCloseButtonClass, authPrimaryButtonClass, authSwitchButtonClass } from './modalUi'

describe('Auth Interactive Control State adapter', () => {
  it.each([
    ['primary action', authPrimaryButtonClass],
    ['switch action', authSwitchButtonClass],
    ['close action', authCloseButtonClass],
  ])('wires the %s to semantic Motion Policy utilities', (_name, adapter) => {
    expect(adapter).toContain('control-state')
    expect(adapter).toContain('duration-[var(--motion-control-duration)]')
    expect(adapter).toContain('ease-[var(--motion-control-easing)]')
  })
})
