type MenuKeyboardOptions = {
  onEscape?: () => void
}

/**
 * Applies the shared roving-focus behavior for a role=menu container.
 * Returns true when the key was consumed by menu navigation.
 */
export function handleMenuKeyboardNavigation(
  event: KeyboardEvent,
  menu: HTMLElement | null,
  options: MenuKeyboardOptions = {},
): boolean {
  if (event.key === 'Escape' && options.onEscape) {
    event.preventDefault()
    options.onEscape()
    return true
  }

  const items = Array.from(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])
  if (!items.length) return false

  let nextIndex: number | null = null
  const currentIndex = Math.max(0, items.indexOf(document.activeElement as HTMLElement))
  if (event.key === 'ArrowDown') nextIndex = (currentIndex + 1) % items.length
  if (event.key === 'ArrowUp') nextIndex = (currentIndex - 1 + items.length) % items.length
  if (event.key === 'Home') nextIndex = 0
  if (event.key === 'End') nextIndex = items.length - 1
  if (nextIndex === null) return false

  event.preventDefault()
  items[nextIndex]?.focus()
  return true
}
