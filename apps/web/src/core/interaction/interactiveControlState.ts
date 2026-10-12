export type InteractiveControlTone = 'default' | 'danger'

export type InteractiveControlStateOptions = {
  isDisabled?: boolean
  pending?: boolean
  isSelected?: boolean
  tone?: InteractiveControlTone
}

export type InteractiveControlState = 'idle' | 'selected' | 'disabled' | 'loading'

export type InteractiveControlStateAttrs = {
  disabled: boolean
  'aria-busy'?: 'true'
  'aria-pressed'?: 'true' | 'false'
  'data-control-state': InteractiveControlState
  'data-control-tone'?: 'danger'
}

export function resolveInteractiveControlState(
  options: InteractiveControlStateOptions = {},
): InteractiveControlStateAttrs {
  const pending = options.pending === true
  const disabled = pending || options.isDisabled === true
  const state: InteractiveControlState = pending
    ? 'loading'
    : disabled
      ? 'disabled'
      : options.isSelected
        ? 'selected'
        : 'idle'

  return {
    disabled,
    ...(pending ? { 'aria-busy': 'true' as const } : {}),
    ...(options.isSelected === undefined
      ? {}
      : { 'aria-pressed': options.isSelected ? ('true' as const) : ('false' as const) }),
    'data-control-state': state,
    ...(options.tone === 'danger' ? { 'data-control-tone': 'danger' as const } : {}),
  }
}
