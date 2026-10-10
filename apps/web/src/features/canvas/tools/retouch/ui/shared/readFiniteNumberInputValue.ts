export function readFiniteNumberInputValue(event: Event) {
  const target = event.target
  if (!(target instanceof HTMLInputElement)) return null

  const value = target.valueAsNumber
  return Number.isFinite(value) ? value : null
}
