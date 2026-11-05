// Member quota locale keys + period values (single source).
//
// All User copy lives in the locale bundles under `workspace.quota.*`; SFCs
// resolve copy through `t()` (four-locale parity). Key strings are frozen.
import type { MemberQuotaPeriod } from '@/features/account/session/quota/resource'

export const MEMBER_QUOTA_TABLE_KEYS = {
  overviewExhausted: 'workspace.quota.overviewExhausted',
  tableLabel: 'workspace.quota.tableLabel',
  headerMember: 'workspace.quota.headerMember',
  headerPeriod: 'workspace.quota.headerPeriod',
  headerUsedTotal: 'workspace.quota.headerUsed',
  headerRemaining: 'workspace.quota.headerRemaining',
  headerStatus: 'workspace.quota.headerStatus',
  headerActions: 'workspace.quota.headerActions',
  emptyTitle: 'workspace.quota.emptyTitle',
  emptyDescription: 'workspace.quota.emptyDescription',
  createFirst: 'workspace.quota.createFirst',
  createAction: 'workspace.quota.createAction',
  adjustAction: 'workspace.quota.adjustAction',
  unlimitedDash: 'workspace.quota.unlimitedDash',
} as const

export type MemberQuotaEditMode = 'create' | 'edit'

export const MEMBER_QUOTA_EDIT_KEYS = {
  editTitle: 'workspace.quota.editTitle',
  createTitle: 'workspace.quota.editCreateTitle',
  description: 'workspace.quota.editDescription',
  memberPlaceholder: 'workspace.quota.memberPlaceholder',
  unlimitedHint: 'workspace.quota.unlimitedHint',
  clearConfirmTitle: 'workspace.quota.clearConfirmTitle',
  clearConfirmMessage: 'workspace.quota.clearConfirmMessage',
  clearConfirmAction: 'workspace.quota.clearConfirmAction',
} as const

export const MEMBER_QUOTA_STATUS_KEYS = {
  nearExhausted: 'workspace.quota.statusNearExhausted',
  unlimited: 'workspace.quota.statusUnlimited',
  exhausted: 'workspace.quota.statusExhausted',
  nearExhaustedHint: 'workspace.quota.nearExhaustedHint',
} as const

export const PERIOD_VALUES: readonly MemberQuotaPeriod[] = [
  'daily',
  'weekly',
  'monthly',
  'fixed',
  'unlimited',
]
