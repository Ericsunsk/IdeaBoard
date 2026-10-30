import { describe, expect, it } from 'vitest'
import {
  boardOperationIdempotencyKey,
  controlCreditIdempotencyKey,
  controlUserIdempotencyKey,
  organizationClosureIdempotencyKey,
  organizationCreationIdempotencyKey,
  taskCancellationIdempotencyKey,
  workspaceBillingCheckoutIdempotencyKey,
  workspaceBillingPendingOrderIdempotencyKey,
  workspaceBillingTransferIdempotencyKey,
  workspaceCreationIdempotencyKey,
  workspaceDeletionIdempotencyKey,
  workspaceLibraryIdempotencyKey,
  workspaceMediaIdempotencyKey,
  workspaceMemberIdempotencyKey,
  workspaceRewardsRedeemIdempotencyKey,
  workspaceTaskIdempotencyKey,
  workspaceUploadIdempotencyKey,
  workspaceVisionQueryIdempotencyKey,
} from './idempotencyKey'

describe('idempotencyKey', () => {
  describe('deterministic task cancellation key', () => {
    it('returns target-keyed deterministic format', () => {
      expect(taskCancellationIdempotencyKey('task-123')).toBe('task-cancel:task-123')
      expect(taskCancellationIdempotencyKey('task-456')).toBe('task-cancel:task-456')
    })
  })

  describe('domain command keys', () => {
    it('mints workspace creation and deletion keys', () => {
      const creationKey = workspaceCreationIdempotencyKey()
      expect(creationKey).toMatch(/^workspace-create:.+/)
      expect(workspaceCreationIdempotencyKey()).not.toBe(creationKey)
      expect(workspaceDeletionIdempotencyKey()).toMatch(/^workspace-delete:.+/)
    })

    it('mints organization creation and closure keys', () => {
      expect(organizationCreationIdempotencyKey()).toMatch(/^organization-create:.+/)
      expect(organizationClosureIdempotencyKey()).toMatch(/^organization:close:.+/)
    })

    it('mints board operation keys with fallback', () => {
      expect(boardOperationIdempotencyKey('save')).toMatch(/^board-save:.+/)
      expect(boardOperationIdempotencyKey('')).toMatch(/^board-write:.+/)
      expect(boardOperationIdempotencyKey('   ')).toMatch(/^board-write:.+/)
    })

    it('mints upload and media keys', () => {
      expect(workspaceUploadIdempotencyKey('create')).toMatch(/^upload:create:.+/)
      expect(workspaceUploadIdempotencyKey('bytes')).toMatch(/^upload:bytes:.+/)
      expect(workspaceUploadIdempotencyKey('complete')).toMatch(/^upload:complete:.+/)
      expect(workspaceMediaIdempotencyKey('archive')).toMatch(/^media:archive:.+/)
    })

    it('mints billing checkout, pending-order, and transfer keys', () => {
      expect(workspaceBillingCheckoutIdempotencyKey()).toMatch(/^billing:checkout:.+/)
      expect(workspaceBillingPendingOrderIdempotencyKey('continue')).toMatch(
        /^billing:pending-continue:.+/,
      )
      expect(workspaceBillingPendingOrderIdempotencyKey('abandon')).toMatch(
        /^billing:pending-abandon:.+/,
      )
      expect(workspaceBillingTransferIdempotencyKey('invite')).toMatch(
        /^billing:transfer-invite:.+/,
      )
      expect(workspaceBillingTransferIdempotencyKey('accept')).toMatch(
        /^billing:transfer-accept:.+/,
      )
      expect(workspaceBillingTransferIdempotencyKey('reject')).toMatch(
        /^billing:transfer-reject:.+/,
      )
      expect(workspaceBillingTransferIdempotencyKey('cancel')).toMatch(
        /^billing:transfer-cancel:.+/,
      )
    })

    it('mints library and vision query keys', () => {
      expect(workspaceLibraryIdempotencyKey('reorder')).toMatch(/^library:reorder:.+/)
      expect(workspaceLibraryIdempotencyKey('')).toMatch(/^library:write:.+/)
      expect(workspaceVisionQueryIdempotencyKey()).toMatch(/^search:vision:.+/)
    })

    it('mints rewards, task, and member keys', () => {
      expect(workspaceRewardsRedeemIdempotencyKey()).toMatch(/^rewards:redeem:.+/)
      expect(workspaceTaskIdempotencyKey('create')).toMatch(/^task:create:.+/)
      expect(workspaceTaskIdempotencyKey('cancel')).toMatch(/^task:cancel:.+/)
      expect(workspaceMemberIdempotencyKey('invite')).toMatch(/^workspace:members\.invite:.+/)
      expect(workspaceMemberIdempotencyKey('remove')).toMatch(/^workspace:members\.remove:.+/)
    })

    it('mints control plane user and credit administration keys', () => {
      expect(controlUserIdempotencyKey('ban')).toMatch(/^control:users\.ban:.+/)
      expect(controlUserIdempotencyKey('unban')).toMatch(/^control:users\.unban:.+/)
      expect(controlUserIdempotencyKey('revoke-sessions')).toMatch(
        /^control:users\.revoke-sessions:.+/,
      )
      expect(controlCreditIdempotencyKey('grant')).toMatch(/^control:credits\.grant:.+/)
      expect(controlCreditIdempotencyKey('refund')).toMatch(/^control:credits\.refund:.+/)
    })
  })
})
