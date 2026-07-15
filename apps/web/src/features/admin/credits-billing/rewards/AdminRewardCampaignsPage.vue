<template>
  <div class="flex min-w-0 flex-col gap-4">
    <section class="border-b border-admin-border-soft bg-ib-surface px-1 pb-5">
      <div class="mb-4">
        <h2 class="m-0 text-admin-title font-[760] text-admin-title-color">奖励活动</h2>
      </div>

      <form class="grid gap-3 md:grid-cols-2 xl:grid-cols-4" @submit.prevent="createCampaign">
        <fieldset class="flex min-w-0 flex-col gap-1.5 border-0 p-0">
          <legend class="text-admin-meta font-[680] text-admin-text-2">
            Code 模式 <span class="text-admin-danger" aria-hidden="true">*</span>
          </legend>
          <SelectButton
            v-model="draft.mode"
            :options="campaignModeOptions"
            option-label="label"
            option-value="value"
            :allow-empty="false"
          />
        </fieldset>
        <RewardCampaignSettingsFields
          v-model:display-name="draft.displayName"
          v-model:internal-note="draft.internalNote"
          v-model:credit-amount="draft.creditAmount"
          v-model:max-redemptions="draft.maxRedemptions"
          v-model:starts-at="draft.startsAt"
          v-model:expires-at="draft.expiresAt"
          id-prefix="reward-create"
          note-label="初始内部备注"
          :min-redemptions="draft.mode === 'single_use' ? 0 : 1"
          :redemption-cap-hint="draft.mode === 'single_use' ? '0 表示无限' : ''"
          :schedule-error="createScheduleError"
        />
        <AdminActionReasonField
          v-model="draft.reason"
          input-id="reward-create-reason"
          label="创建原因"
          body-class="md:col-span-2"
          :rows="2"
        />
        <div class="flex items-end xl:col-span-4">
          <Button
            type="submit"
            :label="draft.mode === 'single_use' ? '创建单次码 Draft' : '创建公共 Draft'"
            :loading="saving"
            :disabled="!canCreate"
          />
        </div>
      </form>
    </section>

    <AdminTablePanel>
      <Message v-if="loadError" severity="error" :closable="false" class="m-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>奖励活动加载失败，请重试。</span>
          <Button label="重试" severity="secondary" size="small" outlined @click="loadCampaigns" />
        </div>
      </Message>
      <DataTableFrame
        v-else
        :value="campaignViews"
        :loading="loading"
        data-key="id"
        table-aria-label="奖励活动列表"
        scrollable
        scroll-aria-label="奖励活动列表横向滚动区域"
        empty-title="暂无奖励活动"
      >
        <Column field="displayName" header="活动">
          <template #body="{ data }">
            <div class="grid min-w-40 gap-0.5">
              <span class="font-[680] text-admin-title-color">{{ data.displayName }}</span>
              <span class="truncate text-admin-caption text-admin-text-3">
                {{ data.internalNote || '无内部备注' }}
              </span>
            </div>
          </template>
        </Column>
        <Column header="状态">
          <template #body="{ data }">
            <div class="flex min-w-24 flex-col items-start gap-1">
              <Tag
                :value="data.statusLabel"
                :severity="data.statusSeverity"
                :class="adminCompactTagClass"
              />
              <span class="text-admin-caption text-admin-text-3">{{ data.lifecycleLabel }}</span>
            </div>
          </template>
        </Column>
        <Column header="Code 模式">
          <template #body="{ data }">{{ data.modeLabel }}</template>
        </Column>
        <Column header="有效时间">
          <template #body="{ data }">
            <span class="whitespace-nowrap text-admin-meta text-admin-text-2">
              {{ data.scheduleLabel }}
            </span>
          </template>
        </Column>
        <Column header="单次奖励">
          <template #body="{ data }">{{ formatIdeaAmount(data.creditAmount) }}</template>
        </Column>
        <Column header="总上限">
          <template #body="{ data }">{{ data.maxRedemptionsLabel }}</template>
        </Column>
        <Column header="Code">
          <template #body="{ data }">
            <div class="grid min-w-24 gap-0.5 text-admin-meta">
              <span>{{ data.issuedCodeCount }} 已发行</span>
              <span class="text-admin-text-3">{{ data.validUnusedCodeCount }} 可用</span>
            </div>
          </template>
        </Column>
        <Column header="兑换进度">
          <template #body="{ data }">
            <div class="grid min-w-28 gap-1.5">
              <span class="text-admin-meta font-[650] text-admin-text-2">
                {{ data.progressLabel }}
              </span>
              <div
                v-if="data.hasRedemptionCap"
                class="h-1.5 overflow-hidden rounded-full bg-admin-border-soft"
                role="progressbar"
                aria-label="兑换进度"
                aria-valuemin="0"
                aria-valuemax="100"
                :aria-valuenow="data.progressPercent"
              >
                <div
                  class="h-full rounded-full bg-admin-success"
                  :style="{ width: `${data.progressPercent}%` }"
                />
              </div>
            </div>
          </template>
        </Column>
        <Column header="操作">
          <template #body="{ data }">
            <Button
              size="small"
              label="管理"
              :aria-label="`管理${data.displayName}`"
              text
              @click="selectCampaign(data)"
            />
          </template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>

    <section v-if="selected && selectedView" class="border-t border-admin-border-soft bg-ib-surface px-1 pt-5">
      <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div class="grid gap-1">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="m-0 text-admin-title-sm font-[740] text-admin-title-color">
              {{ selected.displayName }}
            </h3>
            <Tag
              :value="selectedView.statusLabel"
              :severity="selectedView.statusSeverity"
              :class="adminCompactTagClass"
            />
          </div>
          <span class="text-admin-caption text-admin-text-3">
            {{ selectedView.lifecycleLabel }} · {{ selectedView.scheduleLabel }}
          </span>
        </div>

        <div class="flex flex-wrap gap-2">
          <Button
            v-if="selected.lifecycleStatus === 'draft' && selected.issuedCodeCount > 0"
            label="激活活动"
            severity="secondary"
            :disabled="saving"
            @click="requestAuditedAction('activate')"
          />
          <Button
            v-if="selected.lifecycleStatus === 'active'"
            label="暂停活动"
            severity="secondary"
            :disabled="saving"
            @click="requestAuditedAction('pause')"
          />
          <Button
            v-if="selected.status === 'paused'"
            label="恢复活动"
            severity="secondary"
            :disabled="saving"
            @click="requestAuditedAction('resume')"
          />
          <Button
            v-if="selected.lifecycleStatus === 'draft' && selected.issuedCodeCount === 0"
            label="删除空 Draft"
            severity="danger"
            outlined
            :disabled="saving"
            @click="requestAuditedAction('delete')"
          />
          <Button
            v-if="selected.lifecycleStatus !== 'cancelled' && selected.issuedCodeCount > 0"
            label="取消活动"
            severity="danger"
            :disabled="saving"
            @click="requestAuditedAction('cancel')"
          />
        </div>
      </div>

      <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,0.42fr)]">
        <form class="grid gap-3 md:grid-cols-2" @submit.prevent="requestUpdate">
          <RewardCampaignSettingsFields
            v-model:display-name="edit.displayName"
            v-model:internal-note="edit.internalNote"
            v-model:credit-amount="edit.creditAmount"
            v-model:max-redemptions="edit.maxRedemptions"
            v-model:starts-at="edit.startsAt"
            v-model:expires-at="edit.expiresAt"
            id-prefix="reward-edit"
            note-label="活动内部备注"
            :min-redemptions="selected.mode === 'single_use' ? 0 : 1"
            :redemption-cap-hint="selected.mode === 'single_use' ? '0 表示无限' : ''"
            :schedule-error="editScheduleError"
          />
          <AdminActionReasonField
            v-model="edit.reason"
            input-id="reward-edit-reason"
            label="更新原因"
            :error="editReasonError"
            body-class="md:col-span-2"
            :rows="2"
            @update:model-value="editReasonError = ''"
          />
          <div class="md:col-span-2">
            <Button type="submit" label="保存活动设置" :loading="saving" :disabled="!canUpdate" />
          </div>
        </form>

        <div
          v-if="selected.mode === 'public' && selected.lifecycleStatus === 'draft'"
          class="grid content-start gap-3 border-l border-admin-border-soft pl-5 max-xl:border-l-0 max-xl:border-t max-xl:pl-0 max-xl:pt-5"
        >
          <AdminFormField label="公共兑换码" for="reward-public-code" required>
            <InputText
              id="reward-public-code"
              v-model="publicCode"
              unstyled
              autocomplete="off"
              :class="ibFormInputClass"
            />
          </AdminFormField>
          <Button
            label="发行公共兑换码"
            :disabled="saving || !publicCode.trim()"
            @click="requestAuditedAction('issue')"
          />
          <Message v-if="issuedPlaintext" severity="success" :closable="false">
            兑换码仅本次显示：<strong>{{ issuedPlaintext }}</strong>
          </Message>
        </div>
      </div>

      <RewardCodeBatchPanel
        v-if="selected.mode === 'single_use'"
        :campaign="selected"
        @changed="loadCampaigns"
      />
    </section>

    <AdminActionReasonDialog
      v-model:visible="actionDialogVisible"
      :title="actionDialog.title"
      :message="actionDialog.message"
      :confirm-label="actionDialog.confirmLabel"
      :intent="actionDialog.intent"
      :loading="saving"
      @confirm="confirmAuditedAction"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import { ibFormInputClass } from '../../../../shared/theme/formControls'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import AdminActionReasonDialog from '../AdminActionReasonDialog.vue'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import RewardCampaignSettingsFields from './RewardCampaignSettingsFields.vue'
import RewardCodeBatchPanel from './RewardCodeBatchPanel.vue'
import {
  getRewardCampaignActionDescriptor,
  type RewardCampaignActionResult,
  type RewardCampaignAuditedAction,
} from './rewardCampaignActions'
import { createRewardCampaignView } from './rewardCampaignPresentation'
import {
  createAdminRewardCampaignResource,
  type CreateRewardCampaignInput,
  type RewardCampaign,
  RewardCampaignConflictError,
  type UpdateRewardCampaignInput,
} from './rewardCampaignResource'
import { isoToShanghaiDateTimeInput } from './rewardCampaignTime'

const resource = createAdminRewardCampaignResource()
const toast = useAdminToast()
const adminConfirm = useAdminConfirm()
const campaigns = ref<RewardCampaign[]>([])
const selected = ref<RewardCampaign | null>(null)
const loading = ref(true)
const loadError = ref(false)
const saving = ref(false)
const issuedPlaintext = ref('')
const publicCode = ref('')
const pendingAction = ref<RewardCampaignAuditedAction | null>(null)
const actionDialogVisible = ref(false)
const editReasonError = ref('')
const campaignModeOptions = [
  { label: '公共码', value: 'public' },
  { label: '单次码', value: 'single_use' },
] satisfies Array<{ label: string; value: CreateRewardCampaignInput['mode'] }>

const draft = reactive<CreateRewardCampaignInput>(emptyCampaignForm())
const edit = reactive<UpdateRewardCampaignInput>(emptyCampaignSettingsForm())

const campaignViews = computed(() => campaigns.value.map(createRewardCampaignView))
const selectedView = computed(() => (selected.value ? createRewardCampaignView(selected.value) : null))
const createScheduleError = computed(() => scheduleError(draft.startsAt, draft.expiresAt))
const editScheduleError = computed(() => scheduleError(edit.startsAt, edit.expiresAt))
const canCreate = computed(
  () =>
    Boolean(draft.displayName.trim()) &&
    draft.creditAmount > 0 &&
    validRedemptionCap(draft.mode, draft.maxRedemptions) &&
    validateAdminActionReason(draft.reason).valid &&
    !createScheduleError.value,
)
const canUpdate = computed(
  () =>
    Boolean(edit.displayName.trim()) &&
    edit.creditAmount > 0 &&
    Boolean(selected.value && validRedemptionCap(selected.value.mode, edit.maxRedemptions)) &&
    !editScheduleError.value,
)
const actionDialog = computed(() =>
  getRewardCampaignActionDescriptor(pendingAction.value ?? 'activate').dialog(
    selected.value?.displayName || '当前活动',
  ),
)

onMounted(loadCampaigns)

async function loadCampaigns() {
  loading.value = true
  loadError.value = false
  try {
    campaigns.value = (await resource.list()).campaigns
    if (selected.value) {
      const refreshed = campaigns.value.find((campaign) => campaign.id === selected.value?.id)
      if (refreshed) selectCampaign(refreshed)
    }
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminDisplayError(error, '加载奖励活动失败'))
  } finally {
    loading.value = false
  }
}

async function createCampaign() {
  if (!canCreate.value) return
  await runCommand(
    async () => {
      const campaign = await resource.create({ ...draft })
      campaigns.value.unshift(campaign)
      selectCampaign(campaign)
      Object.assign(draft, emptyCampaignForm())
      toast.success('奖励活动已创建')
    },
    '创建奖励活动失败',
  )
}

function selectCampaign(campaign: RewardCampaign) {
  selected.value = campaign
  issuedPlaintext.value = ''
  publicCode.value = ''
  editReasonError.value = ''
  Object.assign(edit, {
    displayName: campaign.displayName,
    internalNote: campaign.internalNote,
    creditAmount: campaign.creditAmount,
    maxRedemptions: campaign.maxRedemptions,
    startsAt: isoToShanghaiDateTimeInput(campaign.startsAt),
    expiresAt: isoToShanghaiDateTimeInput(campaign.expiresAt),
    reason: '',
  })
}

function requestUpdate() {
  if (!selected.value || !canUpdate.value) return
  const reasonValidation = validateAdminActionReason(edit.reason)
  if (!reasonValidation.valid) {
    editReasonError.value = reasonValidation.error
    return
  }
  const campaign = selected.value
  adminConfirm.request({
    title: '确认更新奖励活动',
    message: `将更新“${campaign.displayName}”的活动设置，已发行兑换码的核心规则不可变。`,
    confirmLabel: '确认更新',
    intent: 'warn',
    onConfirm: async () => {
      await runCommand(
        async () => {
          const updated = await resource.update(campaign.id, {
            ...edit,
            reason: reasonValidation.normalizedReason,
          })
          replaceCampaign(updated)
          selectCampaign(updated)
          toast.success('奖励活动已更新')
        },
        '更新奖励活动失败',
        'update',
      )
    },
  })
}

function requestAuditedAction(action: RewardCampaignAuditedAction) {
  if (!selected.value || saving.value) return
  if (action === 'issue' && !publicCode.value.trim()) {
    toast.error('请输入公共兑换码')
    return
  }
  pendingAction.value = action
  actionDialogVisible.value = true
}

async function confirmAuditedAction(reason: string) {
  if (!selected.value || !pendingAction.value) return
  const action = pendingAction.value
  const campaign = selected.value
  const descriptor = getRewardCampaignActionDescriptor(action)
  const succeeded = await runCommand(
    async () => {
      const result = await descriptor.execute({
        resource,
        campaign,
        reason,
        publicCode: publicCode.value,
      })
      applyAuditedActionResult(campaign, result)
      toast.success(descriptor.successMessage)
    },
    descriptor.errorFallback,
    action,
  )
  if (succeeded) {
    pendingAction.value = null
    actionDialogVisible.value = false
  }
}

function applyAuditedActionResult(
  campaign: RewardCampaign,
  result: RewardCampaignActionResult,
) {
  switch (result.type) {
    case 'campaign':
      replaceCampaign(result.campaign)
      break
    case 'issued':
      issuedPlaintext.value = result.plaintext
      break
    case 'deleted':
      campaigns.value = campaigns.value.filter((item) => item.id !== campaign.id)
      selected.value = null
      break
  }
}

function replaceCampaign(campaign: RewardCampaign) {
  const index = campaigns.value.findIndex((item) => item.id === campaign.id)
  if (index >= 0) campaigns.value[index] = campaign
  selected.value = campaign
}

async function runCommand(
  command: () => Promise<void>,
  fallback: string,
  action?: RewardCampaignAuditedAction | 'update',
) {
  if (saving.value) return false
  saving.value = true
  try {
    await command()
    return true
  } catch (error) {
    const message = formatAdminDisplayError(error, fallback)
    if (
      action === 'update' &&
      error instanceof RewardCampaignConflictError &&
      error.kind === 'rules_frozen'
    ) {
      toast.error('活动已发行兑换码，仅内部备注可以修改')
    } else if (
      action === 'delete' &&
      error instanceof RewardCampaignConflictError &&
      error.kind === 'draft_not_empty'
    ) {
      toast.error('仅未发行兑换码的空 Draft 可以删除')
    } else {
      toast.error(message)
    }
    return false
  } finally {
    saving.value = false
  }
}

function scheduleError(startsAt: string, expiresAt: string) {
  return startsAt && expiresAt && startsAt >= expiresAt ? '结束时间必须晚于开始时间' : ''
}

function validRedemptionCap(mode: RewardCampaign['mode'], value: number) {
  return Number.isSafeInteger(value) && value >= (mode === 'single_use' ? 0 : 1)
}

function emptyCampaignSettingsForm(): UpdateRewardCampaignInput {
  return {
    displayName: '',
    internalNote: '',
    creditAmount: 0,
    maxRedemptions: 0,
    startsAt: '',
    expiresAt: '',
    reason: '',
  }
}

function emptyCampaignForm(): CreateRewardCampaignInput {
  return {
    ...emptyCampaignSettingsForm(),
    mode: 'public',
  }
}
</script>
