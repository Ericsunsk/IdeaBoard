<template>
  <section class="mt-5 border-t border-admin-border-soft pt-5">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h4 class="m-0 text-admin-title-sm font-[720] text-admin-title-color">Code Batches</h4>
        <p
          v-if="batchGenerationUnavailableReason"
          id="reward-batch-generation-unavailable"
          class="mb-0 mt-1 text-admin-caption text-admin-text-3"
        >
          {{ batchGenerationUnavailableReason }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <AdminPagerControls
          :current-page="pagination.currentPage.value"
          :total-pages="pagination.knownPageCount.value"
          :prev-disabled="!pagination.canRetreat.value"
          :next-disabled="!pagination.canAdvance.value"
          @prev="pagination.retreat"
          @next="pagination.advance"
        />
        <Button
          label="生成 Batch"
          :disabled="mutationPending || Boolean(batchGenerationUnavailableReason)"
          :aria-describedby="
            batchGenerationUnavailableReason ? 'reward-batch-generation-unavailable' : undefined
          "
          @click="openGenerateDialog()"
        />
      </div>
    </div>

    <Message v-if="exportUnavailableBatch" severity="warn" :closable="false" class="mb-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>Batch #{{ exportUnavailableBatch.id }} 的明文无法再次导出。</span>
        <Button
          v-if="exportUnavailableBatch.validUnusedCount > 0"
          label="撤销并生成替代 Batch"
          severity="danger"
          outlined
          size="small"
          :disabled="mutationPending"
          @click="requestBatchRevoke(exportUnavailableBatch, true)"
        />
        <span v-else class="text-admin-caption text-admin-text-3">
          无未使用 Code，无需生成替代 Batch
        </span>
      </div>
    </Message>

    <AdminTablePanel>
      <Message v-if="loadError" severity="error" :closable="false" class="m-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>Code Batch 加载失败，请重试。</span>
          <Button label="重试" severity="secondary" size="small" outlined @click="loadBatches" />
        </div>
      </Message>
      <DataTableFrame
        v-else
        :value="batches"
        :loading="loading"
        data-key="id"
        table-aria-label="Code Batch 列表"
        scrollable
        scroll-aria-label="Code Batch 列表横向滚动区域"
        empty-title="暂无 Code Batch"
      >
        <Column header="Batch">
          <template #body="{ data }">
            <div class="grid gap-0.5">
              <span class="font-[680] text-admin-title-color">Batch #{{ data.id }}</span>
              <span class="text-admin-caption text-admin-text-3">
                {{ formatShanghaiDateTime(data.issuedAt) }}
              </span>
            </div>
          </template>
        </Column>
        <Column header="发行量">
          <template #body="{ data }">{{ data.codeCount }} 个</template>
        </Column>
        <Column header="当前计数">
          <template #body="{ data }">
            <div class="flex min-w-40 flex-wrap gap-x-3 gap-y-1 text-admin-meta">
              <span class="text-admin-success">{{ data.validUnusedCount }} 可用</span>
              <span class="text-admin-text-2">{{ data.redeemedCount }} 已兑换</span>
              <span class="text-admin-text-3">{{ data.revokedUnusedCount }} 已撤销</span>
            </div>
          </template>
        </Column>
        <Column header="状态">
          <template #body="{ data }">
            <Tag
              :value="batchStatus(data).label"
              :severity="batchStatus(data).severity"
              :class="adminCompactTagClass"
            />
          </template>
        </Column>
        <Column header="操作">
          <template #body="{ data }">
            <div v-if="data.validUnusedCount > 0" class="flex items-center gap-1">
              <Button
                label="导出丢失"
                :aria-label="`Batch #${data.id} 导出丢失`"
                severity="secondary"
                text
                size="small"
                :disabled="mutationPending"
                @click="requestBatchRevoke(data, true)"
              />
              <Button
                label="撤销未使用 Code"
                :aria-label="`撤销 Batch #${data.id} 未使用 Code`"
                severity="danger"
                text
                size="small"
                :disabled="mutationPending"
                @click="requestBatchRevoke(data, false)"
              />
            </div>
            <span v-else class="text-admin-caption text-admin-text-3">无可撤销 Code</span>
          </template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>

    <AdminDialogShell
      v-model:visible="generateDialogVisible"
      :header="generateDialogTitle"
      size="compact"
      :busy="mutationPending"
    >
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Code 数量"
        for="reward-batch-count"
        :error="showGenerateErrors ? batchCountError : ''"
        required
      >
        <InputNumber
          v-model="generateForm.count"
          input-id="reward-batch-count"
          :min="1"
          :max="batchGenerationMaxCount"
          :use-grouping="false"
          fluid
          :invalid="invalid"
          :aria-describedby="describedbyId"
        />
      </AdminFormField>
      <AdminActionReasonField
        v-model="generateForm.reason"
        input-id="reward-batch-generate-reason"
        label="生成原因"
        :error="showGenerateErrors ? generateReasonValidation.error : ''"
        @update:model-value="showGenerateErrors = false"
      />

      <template #footer>
        <Button
          label="取消"
          severity="secondary"
          outlined
          :disabled="mutationPending"
          @click="generateDialogVisible = false"
        />
        <Button label="确认生成" :loading="mutationPending" @click="generateBatch" />
      </template>
    </AdminDialogShell>

    <AdminActionReasonDialog
      v-model:visible="revokeDialogVisible"
      title="确认撤销 Code Batch"
      :message="revokeDialogMessage"
      confirm-label="确认撤销"
      intent="danger"
      :loading="mutationPending"
      @confirm="confirmBatchRevoke"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { useAdminToast, formatAdminDisplayError } from '../../shared/lib/adminToast'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import AdminActionReasonDialog from '../AdminActionReasonDialog.vue'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import type { RewardCampaign } from './rewardCampaignResource'
import {
  createAdminRewardCodeBatchResource,
  type RewardCodeBatch,
} from './rewardCodeBatchResource'
import { createRewardCodeBatchCsv, downloadRewardCodeBatchCsv } from './rewardCodeBatchCsv'
import { createRewardCodeBatchIdempotencyKey } from './rewardCodeBatchIdempotency'
import { formatShanghaiDateTime } from './rewardCampaignTime'

const props = defineProps<{ campaign: RewardCampaign }>()
const emit = defineEmits<{ changed: [] }>()

const resource = createAdminRewardCodeBatchResource()
const toast = useAdminToast()
const batches = ref<RewardCodeBatch[]>([])
const loading = ref(false)
const loadError = ref(false)
const mutationPending = ref(false)
const generateDialogVisible = ref(false)
const generateDialogTitle = ref('生成 Code Batch')
const showGenerateErrors = ref(false)
const exportUnavailableBatch = ref<RewardCodeBatch | null>(null)
const revokeDialogVisible = ref(false)
const pendingRevoke = ref<{ batch: RewardCodeBatch; prepareReplacement: boolean } | null>(null)
const generateForm = reactive({ count: 100, idempotencyKey: '', reason: '' })
const localValidUnusedAdjustment = ref(0)
const pagination = useAdminCursorPagination<number>({ pending: loading })
const projectedValidUnusedCount = computed(() =>
  Math.max(0, props.campaign.validUnusedCodeCount + localValidUnusedAdjustment.value),
)
const batchGenerationMaxCount = computed(() => {
  if (props.campaign.maxRedemptions === 0) return 10000
  const remaining =
    props.campaign.maxRedemptions -
    props.campaign.redemptionCount -
    projectedValidUnusedCount.value
  return Math.max(0, Math.min(10000, remaining))
})
const batchCountError = computed(() =>
  Number.isSafeInteger(generateForm.count) &&
  generateForm.count >= 1 &&
  generateForm.count <= batchGenerationMaxCount.value
    ? ''
    : `Code 数量必须在 1 到 ${batchGenerationMaxCount.value} 之间`,
)
const generateReasonValidation = computed(() => validateAdminActionReason(generateForm.reason))
const batchGenerationUnavailableReason = computed(() => {
  if (props.campaign.status === 'expired') return '活动已过期，不能追加 Batch'
  if (props.campaign.lifecycleStatus === 'cancelled') return '活动已取消，不能追加 Batch'
  if (batchGenerationMaxCount.value === 0) {
    return '活动总兑换上限已耗尽，不能追加 Batch'
  }
  return ''
})
const revokeDialogMessage = computed(() => {
  const batch = pendingRevoke.value?.batch
  return batch
    ? `将不可逆地撤销 Batch #${batch.id} 的 ${batch.validUnusedCount} 个未使用 Code。`
    : '将不可逆地撤销未使用 Code。'
})

onMounted(loadBatches)
watch(
  () => props.campaign.id,
  () => {
    pagination.reset()
    localValidUnusedAdjustment.value = 0
    exportUnavailableBatch.value = null
    void loadBatches()
  },
)
watch(
  () => props.campaign.validUnusedCodeCount,
  () => {
    localValidUnusedAdjustment.value = 0
  },
)
watch(pagination.cursor, () => void loadBatches())

async function loadBatches() {
  loading.value = true
  loadError.value = false
  try {
    const page = await resource.list(props.campaign.id, pagination.cursor.value)
    batches.value = page.batches
    pagination.setNextCursor(page.nextBeforeId)
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminDisplayError(error, '加载 Code Batch 失败'))
  } finally {
    loading.value = false
  }
}

function openGenerateDialog(count = 100, replacement = false) {
  if (batchGenerationUnavailableReason.value) return
  generateForm.count = Math.min(count, batchGenerationMaxCount.value)
  generateForm.idempotencyKey = createRewardCodeBatchIdempotencyKey()
  generateForm.reason = ''
  showGenerateErrors.value = false
  generateDialogTitle.value = replacement ? '生成替代 Batch' : '生成 Code Batch'
  generateDialogVisible.value = true
}

async function generateBatch() {
  showGenerateErrors.value = true
  if (
    batchGenerationUnavailableReason.value ||
    batchCountError.value ||
    !generateReasonValidation.value.valid ||
    mutationPending.value
  ) {
    return
  }
  mutationPending.value = true
  try {
    const result = await resource.generate(props.campaign.id, {
      count: generateForm.count,
      idempotencyKey: generateForm.idempotencyKey,
      reason: generateReasonValidation.value.normalizedReason,
    })
    upsertBatch(result.batch)
    if (!result.replayed) localValidUnusedAdjustment.value += result.batch.validUnusedCount
    generateDialogVisible.value = false
    if (!result.exportUnavailable && result.plaintexts.length > 0) {
      const csv = createRewardCodeBatchCsv({
        codes: result.plaintexts,
        campaignName: props.campaign.displayName,
        creditAmount: props.campaign.creditAmount,
        expiresAt: props.campaign.expiresAt || '',
      })
      downloadRewardCodeBatchCsv({
        campaignId: props.campaign.id,
        batchId: result.batch.id,
        csv,
      })
      exportUnavailableBatch.value = null
      toast.success('Code Batch 已生成并导出')
    } else {
      exportUnavailableBatch.value = result.batch
      toast.warning('Batch 已存在，明文无法再次导出')
    }
    generateForm.idempotencyKey = ''
    generateForm.reason = ''
    emit('changed')
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '生成 Code Batch 失败'))
  } finally {
    mutationPending.value = false
  }
}

function requestBatchRevoke(batch: RewardCodeBatch, prepareReplacement: boolean) {
  pendingRevoke.value = { batch, prepareReplacement }
  revokeDialogVisible.value = true
}

async function confirmBatchRevoke(reason: string) {
  if (!pendingRevoke.value || mutationPending.value) return
  const target = pendingRevoke.value
  const replacementCount = target.batch.validUnusedCount
  mutationPending.value = true
  try {
    const revoked = await resource.revoke(props.campaign.id, target.batch.id, reason)
    upsertBatch(revoked)
    localValidUnusedAdjustment.value -= replacementCount
    if (exportUnavailableBatch.value?.id === revoked.id) exportUnavailableBatch.value = null
    revokeDialogVisible.value = false
    pendingRevoke.value = null
    toast.success('Batch 未使用 Code 已撤销')
    emit('changed')
    if (target.prepareReplacement && replacementCount > 0) {
      await nextTick()
      openGenerateDialog(replacementCount, true)
    }
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '撤销 Code Batch 失败'))
  } finally {
    mutationPending.value = false
  }
}

function upsertBatch(batch: RewardCodeBatch) {
  const index = batches.value.findIndex((item) => item.id === batch.id)
  if (index >= 0) {
    batches.value[index] = batch
    return
  }
  batches.value = [batch, ...batches.value]
}

function batchStatus(batch: RewardCodeBatch) {
  if (batch.revokedAt) return { label: '已撤销', severity: 'danger' as const }
  if (batch.validUnusedCount === 0) return { label: '已用尽', severity: 'secondary' as const }
  return { label: '可用', severity: 'success' as const }
}
</script>
