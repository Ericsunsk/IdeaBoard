import type { UserAccountMenuWorkspaceItem } from '@/features/account/ui/chrome/userAccountMenu.types'

// Shared workspace-row predicate for the avatar dropdown: the hover
// submenu (in `UserAccountMenu`) and the gear settings dialog show the same
// rows, so the predicate lives here instead of being duplicated.
export function isDefaultWorkspaceItem(
  item: UserAccountMenuWorkspaceItem | { isDefault?: boolean },
): boolean {
  return item.isDefault === true
}
