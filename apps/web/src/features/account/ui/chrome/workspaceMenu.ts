import type { UserAccountMenuWorkspaceItem } from './userAccountMenu.types'

// Shared workspace-row predicates for the avatar dropdown: the hover
// submenu (in `UserAccountMenu`) and the gear settings dialog show the same
// visible set, so the filter lives here instead of being duplicated.
function isVisibleWorkspaceItem(item: UserAccountMenuWorkspaceItem): boolean {
  return item.archivedAt == null
}

export function isDefaultWorkspaceItem(item: UserAccountMenuWorkspaceItem): boolean {
  return item.isDefault === true
}

export function visibleWorkspaceItems(
  items: readonly UserAccountMenuWorkspaceItem[],
): UserAccountMenuWorkspaceItem[] {
  return items.filter(isVisibleWorkspaceItem)
}
