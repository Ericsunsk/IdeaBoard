export type UserAccountMenuItemKey = 'profile' | 'admin' | 'home' | 'logout' | 'login' | 'register'

export type UserAccountMenuItemTone = 'default' | 'danger'

export type UserAccountMenuItem = {
  key: UserAccountMenuItemKey
  label: string
  tone?: UserAccountMenuItemTone
  command: () => void
}

// Workspace switcher row (avatar dropdown): dumb view-model owned by the
// workspace shell. The menu never imports the workspace feature — headers
// (workspace + admin) build this object through `useWorkspaceSwitcher` and
// pass it down, so the row renders identically on both surfaces.
export type UserAccountMenuWorkspaceItem = {
  publicId: string
  name: string
  organizationId: string | null
  // The server marker controls the Default badge and protected actions.
  isDefault: boolean
  archivedAt: string | null
}

type UserAccountMenuWorkspaceStatus = 'loading' | 'ready' | 'error' | 'empty'

export type UserAccountMenuWorkspace = {
  items: UserAccountMenuWorkspaceItem[]
  selectedId: string | null
  status: UserAccountMenuWorkspaceStatus
  /** User-facing load error; shown with a retry affordance. */
  errorMessage?: string | undefined
  /** Inline rename/create ceiling; defaults to the workspace contract (120). */
  nameMaxLength?: number | undefined
  /**
   * Operator's User public identity (`usr_`), matched against each
   * workspace roster to gate member management. Null while anonymous or
   * unresolved: member sections render read-only.
   */
  currentUserId?: string | null | undefined
  onSelect: (workspaceId: string) => void
  onCreate: (name: string) => Promise<UserAccountMenuWorkspaceItem>
  onRename: (workspaceId: string, name: string) => Promise<UserAccountMenuWorkspaceItem>
  onArchive: (workspaceId: string) => Promise<UserAccountMenuWorkspaceItem>
  onRetry: () => void
}

type UserIdeaBalanceControlBase = {
  displayValue: string
  ariaLabel: string
  command: () => void
}

export type UserIdeaBalanceControl =
  | (UserIdeaBalanceControlBase & { status: 'loading' })
  | (UserIdeaBalanceControlBase & { status: 'ready'; availableIdeas: number })
  | (UserIdeaBalanceControlBase & { status: 'error'; errorMessage: string })
  | (Omit<UserIdeaBalanceControlBase, 'displayValue' | 'command'> & {
      status: 'anonymous'
      displayValue?: string
      command?: () => void
    })
