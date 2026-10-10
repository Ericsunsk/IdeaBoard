import { describe, expect, it } from 'vitest'
import { resolveInteractiveControlState } from './interactiveControlState'

describe('resolveInteractiveControlState', () => {
  it('keeps an available control interactive', () => {
    expect(resolveInteractiveControlState()).toEqual({
      disabled: false,
      'data-control-state': 'idle',
    })
  })

  it('makes canonical pending state busy and unavailable', () => {
    expect(resolveInteractiveControlState({ pending: true })).toEqual({
      disabled: true,
      'aria-busy': 'true',
      'data-control-state': 'loading',
    })
  })

  it('exposes stable selected and destructive semantics without choosing surface colors', () => {
    expect(resolveInteractiveControlState({ selected: true, tone: 'danger' })).toEqual({
      disabled: false,
      'aria-pressed': 'true',
      'data-control-state': 'selected',
      'data-control-tone': 'danger',
    })
  })

  it('keeps explicit disabled state distinct from loading', () => {
    expect(resolveInteractiveControlState({ disabled: true, selected: false })).toEqual({
      disabled: true,
      'aria-pressed': 'false',
      'data-control-state': 'disabled',
    })
  })

  it('gives pending precedence over disabled and selected state', () => {
    expect(
      resolveInteractiveControlState({ pending: true, disabled: true, selected: true }),
    ).toEqual({
      disabled: true,
      'aria-busy': 'true',
      'aria-pressed': 'true',
      'data-control-state': 'loading',
    })
  })

  it('gives disabled precedence over selected state', () => {
    expect(resolveInteractiveControlState({ disabled: true, selected: true })).toEqual({
      disabled: true,
      'aria-pressed': 'true',
      'data-control-state': 'disabled',
    })
  })
})
