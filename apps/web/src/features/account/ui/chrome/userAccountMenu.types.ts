import type { Component } from 'vue'

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
}

type UserAccountMenuWorkspaceStatus = 'loading' | 'ready' | 'error' | 'empty'

export type UserAccountMenuWorkspaceCreateInput = {
  name: string
  /** Organization creates always use the organization pool. */
  organizationId?: string | undefined
}

export type UserAccountMenuOrganizationItem = {
  publicId: string
  name: string
  status?: 'active' | 'closed' | undefined
  closedAt?: string | undefined
}

export type UserAccountMenuOrganizationMember = {
  userId: string
  role: 'owner' | 'admin' | 'member'
  createdAt?: string | undefined
}

export type UserAccountMenuOrganizationMemberRole = 'member' | 'admin'

// Organization management panel (issue #951): the workspace shell owns the
// management UI component and its transport callbacks; the account menu only
// carries them through as an opaque `Component` plus plain functions so the
// account feature never imports the workspace feature. The settings dialog
// resolves the caller's single organization and renders the panel, exposing
// the management surface as soon as the organization exists.
export type UserAccountMenuOrganizationPanel = {
  component: Component
  onLoadMembers: (organizationId: string) => Promise<UserAccountMenuOrganizationMember[]>
  onInviteMember: (
    organizationId: string,
    input: { userId: string; role: UserAccountMenuOrganizationMemberRole },
  ) => Promise<UserAccountMenuOrganizationMember>
  onUpdateMember: (
    organizationId: string,
    memberId: string,
    input: { role: UserAccountMenuOrganizationMemberRole },
  ) => Promise<UserAccountMenuOrganizationMember>
  onRemoveMember: (organizationId: string, memberId: string) => Promise<void>
  // Self-leave also resets the Personal/Organization scope and refreshes the
  // list (defaults to onRemoveMember when omitted).
  onLeaveMember?: ((organizationId: string, memberId: string) => Promise<void>) | undefined
  // Permanent Organization closure (owner only). Access and spending end;
  // unused balance cannot be restored or transferred to Personal.
  onCloseOrganization?:
    | ((
        organizationId: string,
        input: { reason?: string | undefined },
      ) => Promise<UserAccountMenuOrganizationItem>)
    | undefined
}

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
  onLoadOrganizations?: (() => Promise<UserAccountMenuOrganizationItem[]>) | undefined
  onCreateOrganization?:
    ((name: string, idempotencyKey: string) => Promise<UserAccountMenuOrganizationItem>) | undefined
  // Organization management panel (issue #951, opaque component + callbacks).
  organizationPanel?: UserAccountMenuOrganizationPanel | undefined
  /** Active Personal/Organization scope (951): null while unresolved. */
  activeScope?: { kind: 'personal' } | { kind: 'organization'; organizationId: string } | undefined
  onSelectPersonal?: (() => void) | undefined
  onSelectOrganization?: ((organizationId: string) => void | Promise<void>) | undefined
  onSelect: (workspaceId: string) => void
  onCreate: (input: UserAccountMenuWorkspaceCreateInput) => Promise<UserAccountMenuWorkspaceItem>
  onRename: (workspaceId: string, name: string) => Promise<UserAccountMenuWorkspaceItem>
  onDelete: (workspaceId: string) => Promise<void>
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
