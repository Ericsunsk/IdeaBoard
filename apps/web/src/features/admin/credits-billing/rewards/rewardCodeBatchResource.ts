import {
  generateAdminRewardCodeBatch,
  listAdminRewardCodeBatches,
  revokeAdminRewardCodeBatch,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  GeneratedRewardCodeBatch as GeneratedBatchResult,
  RewardCodeBatch as GeneratedRewardCodeBatch,
  RewardCodeBatchPage as GeneratedRewardCodeBatchPage,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'

export type RewardCodeBatch = {
  id: number
  campaignId: number
  idempotencyKey: string
  codeCount: number
  validUnusedCount: number
  redeemedCount: number
  revokedUnusedCount: number
  issuedBy: number
  issuedAt: string
  revokedAt?: string
  revokedBy?: number
}

export type RewardCodeBatchPage = {
  batches: RewardCodeBatch[]
  nextBeforeId?: number
}

export type GenerateRewardCodeBatchInput = {
  count: number
  idempotencyKey: string
  reason: string
}

export type GeneratedRewardCodeBatchResult = {
  batch: RewardCodeBatch
  plaintexts: string[]
  exportUnavailable: boolean
  replayed: boolean
}

export type AdminRewardCodeBatchResource = {
  list: (campaignId: number, beforeId?: number) => Promise<RewardCodeBatchPage>
  generate: (
    campaignId: number,
    input: GenerateRewardCodeBatchInput,
  ) => Promise<GeneratedRewardCodeBatchResult>
  revoke: (campaignId: number, batchId: number, reason: string) => Promise<RewardCodeBatch>
}

export function createAdminRewardCodeBatchResource(): AdminRewardCodeBatchResource {
  return {
    list: async (campaignId, beforeId) =>
      toBatchPage(
        await resolveApiData(
          listAdminRewardCodeBatches(campaignId, { before_id: beforeId, limit: 20 }),
          '加载 Code Batch 失败',
        ),
      ),
    generate: async (campaignId, input) =>
      toGeneratedBatchResult(
        await resolveApiData(
          generateAdminRewardCodeBatch(campaignId, {
            count: input.count,
            idempotency_key: input.idempotencyKey.trim(),
            reason: input.reason.trim(),
          }),
          '生成 Code Batch 失败',
        ),
      ),
    revoke: async (campaignId, batchId, reason) =>
      toBatch(
        await resolveApiData(
          revokeAdminRewardCodeBatch(campaignId, batchId, {
            confirmed: true,
            reason: reason.trim(),
          }),
          '撤销 Code Batch 失败',
        ),
      ),
  }
}

function toBatch(batch: GeneratedRewardCodeBatch): RewardCodeBatch {
  return {
    id: batch.id,
    campaignId: batch.campaign_id,
    idempotencyKey: batch.idempotency_key,
    codeCount: batch.code_count,
    validUnusedCount: batch.valid_unused_count,
    redeemedCount: batch.redeemed_count,
    revokedUnusedCount: batch.revoked_unused_count,
    issuedBy: batch.issued_by,
    issuedAt: batch.issued_at,
    revokedAt: batch.revoked_at,
    revokedBy: batch.revoked_by,
  }
}

function toBatchPage(page: GeneratedRewardCodeBatchPage): RewardCodeBatchPage {
  return {
    batches: page.batches.map(toBatch),
    nextBeforeId: page.next_before_id,
  }
}

function toGeneratedBatchResult(result: GeneratedBatchResult): GeneratedRewardCodeBatchResult {
  return {
    batch: toBatch(result.batch),
    plaintexts: result.plaintexts ? [...result.plaintexts] : [],
    exportUnavailable: result.export_unavailable,
    replayed: result.replayed,
  }
}
