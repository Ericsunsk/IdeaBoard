import type { UserAccountMenuWorkspaceItem } from './userAccountMenu.types'

// Shared workspace-row predicate for the avatar dropdown: the hover
// submenu (in `UserAccountMenu`) and the gear settings dialog show the same
// rows, so the predicate lives here instead of being duplicated.
export function isDefaultWorkspaceItem(item: UserAccountMenuWorkspaceItem): boolean {
  return item.isDefault === true
}
