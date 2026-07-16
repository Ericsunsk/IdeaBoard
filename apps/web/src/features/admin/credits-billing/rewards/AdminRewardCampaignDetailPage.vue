<template>
  <div class="flex min-w-0 flex-col gap-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="flex min-w-0 items-start gap-3">
        <Button text aria-label="返回奖励活动列表" @click="goBack">
          <template #icon><NavArrowLeft class="size-5" /></template>
        </Button>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="m-0 text-admin-title font-[760] text-admin-title-color">
              {{ campaign?.displayName || '奖励活动详情' }}
            </h2>
            <Tag
              v-if="campaignView"
              :value="campaignView.statusLabel"
              :severity="campaignView.statusSeverity"
              :class="adminCompactTagClass"
            />
          </div>
          <div v-if="campaignView" class="mt-1 text-admin-caption text-admin-text-3">
            Campaign #{{ campaignView.id }} · {{ campaignView.modeLabel }} ·
            {{ campaignView.scheduleLabel }}
          </div>
        </div>
      </div>
      <div v-if="campaign" class="flex flex-wrap gap-2">
        <Button
          v-if="campaign.lifecycleStatus === 'draft' && campaign.issuedCodeCount > 0"
          label="激活活动"
          severity="secondary"
          :disabled="saving"
          @click="requestAuditedAction('activate')"
        />
        <Button
          v-if="campaign.lifecycleStatus === 'active'"
          label="暂停活动"
          severity="secondary"
          :disabled="saving"
          @click="requestAuditedAction('pause')"
        />
        <Button
          v-if="campaign.status === 'paused'"
          label="恢复活动"
          severity="secondary"
          :disabled="saving"
          @click="requestAuditedAction('resume')"
        />
        <Button
          v-if="campaign.lifecycleStatus === 'draft' && campaign.issuedCodeCount === 0"
          label="删除空 Draft"
          severity="danger"
          outlined
          :disabled="saving"
          @click="requestAuditedAction('delete')"
        />
        <Button
          v-if="campaign.lifecycleStatus !== 'cancelled' && campaign.issuedCodeCount > 0"
          label="取消活动"
          severity="danger"
          :disabled="saving"
          @click="requestAuditedAction('cancel')"
        />
      </div>
    </div>

    <Message v-if="loadError" severity="error" :closable="false">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>奖励活动详情加载失败。</span>
        <Button label="重试" severity="secondary" outlined @click="loadCampaign" />
      </div>
    </Message>

    <ProgressSpinner v-else-if="loading" class="mx-auto my-16 size-8" stroke-width="4" />

    <Tabs v-else-if="campaign && campaignView" v-model:value="activeTab" class="min-w-0">
      <TabList>
        <Tab value="overview">Overview</Tab>
        <Tab value="batches">Code Batches</Tab>
        <Tab value="redemptions">Redemptions</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="overview">
          <div class="flex min-w-0 flex-col gap-5 pt-2">
            <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <div
                v-for="item in overviewItems"
                :key="item.label"
                class="min-w-0 border-b border-admin-border-soft pb-3"
              >
                <div class="text-admin-caption text-admin-text-3">{{ item.label }}</div>
                <div class="mt-1 break-words text-admin-content font-[720] text-admin-text-1">
                  {{ item.value }}
                </div>
              </div>
            </section>

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
                :min-redemptions="campaign.mode === 'single_use' ? 0 : 1"
                :redemption-cap-hint="campaign.mode === 'single_use' ? '0 表示无限' : ''"
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
                <Button
                  type="submit"
                  label="保存活动设置"
                  :loading="saving"
                  :disabled="!canUpdate"
                />
              </div>
            </form>

            <section
              v-if="campaign.mode === 'public' && campaign.lifecycleStatus === 'draft'"
              class="grid max-w-xl gap-3 border-t border-admin-border-soft pt-5"
            >
              <AdminFormField label="公共兑换码" for="reward-public-code" required>
                <InputText id="reward-public-code" v-model="publicCode" autocomplete="off" fluid />
              </AdminFormField>
              <div>
                <Button
                  label="发行公共兑换码"
                  :disabled="saving || !publicCode.trim()"
                  @click="requestAuditedAction('issue')"
                />
              </div>
              <Message v-if="issuedPlaintext" severity="success" :closable="false"
                >兑换码仅本次显示：<strong>{{ issuedPlaintext }}</strong></Message
              >
            </section>
          </div>
        </TabPanel>

        <TabPanel value="batches">
          <div class="flex min-w-0 flex-col gap-5 pt-2">
            <section class="grid gap-3 border-b border-admin-border-soft pb-5">
              <div class="flex flex-wrap items-end gap-2">
                <AdminFormField
                  label="精确查找兑换码"
                  for="reward-code-lookup"
                  body-class="min-w-[280px] flex-1"
                >
                  <InputText
                    id="reward-code-lookup"
                    v-model="codeLookupInput"
                    autocomplete="off"
                    fluid
                    @keydown.enter.prevent="lookupCode"
                  />
                </AdminFormField>
                <Button
                  label="查找"
                  :loading="codeLookupLoading"
                  :disabled="!codeLookupInput.trim()"
                  @click="lookupCode"
                />
              </div>

              <Message v-if="codeLookupError" severity="error" :closable="false">{{
                codeLookupError
              }}</Message>
              <DataTableFrame
                v-if="locatedCode"
                :value="[locatedCode]"
                data-key="id"
                table-aria-label="兑换码精确查找结果"
                scrollable
                table-style="min-width: 760px"
              >
                <Column header="Code"
                  ><template #body="{ data }"
                    ><span class="font-mono font-[700]">{{ data.maskedCodeHint }}</span></template
                  ></Column
                >
                <Column header="活动"
                  ><template #body="{ data }">
                    <div class="flex items-center gap-2">
                      <span>{{ data.campaignDisplayName }} · #{{ data.campaignId }}</span>
                      <Button
                        v-if="data.campaignId !== campaignId"
                        label="查看"
                        text
                        size="small"
                        @click="openLocatedCampaign(data.campaignId)"
                      /></div></template
                ></Column>
                <Column header="Batch"
                  ><template #body="{ data }">{{
                    data.batchId ? `#${data.batchId}` : '公共码'
                  }}</template></Column
                >
                <Column header="状态"
                  ><template #body="{ data }"
                    ><Tag
                      :value="codeStatusLabel(data.status)"
                      :severity="codeStatusSeverity(data.status)"
                      :class="adminCompactTagClass" /></template
                ></Column>
                <Column header="发行时间"
                  ><template #body="{ data }">{{
                    formatShanghaiDateTime(data.issuedAt)
                  }}</template></Column
                >
                <Column header="操作"
                  ><template #body="{ data }"
                    ><Button
                      v-if="data.status === 'available'"
                      label="撤销"
                      severity="danger"
                      text
                      size="small"
                      @click="requestCodeRevoke(data)"
                    /><span v-else class="text-admin-caption text-admin-text-3"
                      >不可撤销</span
                    ></template
                  ></Column
                >
              </DataTableFrame>
            </section>

            <RewardCodeBatchPanel
              v-if="campaign.mode === 'single_use'"
              :campaign="campaign"
              @changed="handleBatchChanged"
            />
            <div v-else class="py-12 text-center text-admin-body text-admin-text-3">
              公共码活动没有 Code Batch
            </div>
          </div>
        </TabPanel>

        <TabPanel value="redemptions">
          <AdminTablePanel content-class="min-h-0" class="mt-2">
            <template #toolbar>
              <AdminResourceToolbar>
                <template #start
                  ><div class="text-admin-title-sm font-[720] text-admin-title-color">
                    兑换记录
                  </div></template
                >
                <template #pager>
                  <AdminPagerControls
                    :current-page="redemptionPagination.currentPage.value"
                    :total-pages="redemptionPagination.knownPageCount.value"
                    :prev-disabled="!redemptionPagination.canRetreat.value"
                    :next-disabled="!redemptionPagination.canAdvance.value"
                    @prev="previousRedemptionPage"
                    @next="nextRedemptionPage"
                  />
                </template>
              </AdminResourceToolbar>
            </template>
            <Message v-if="redemptionError" class="m-5" severity="error" :closable="false">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span>兑换记录加载失败，请重试。</span>
                <Button
                  label="重试"
                  severity="secondary"
                  outlined
                  size="small"
                  @click="loadRedemptions"
                />
              </div>
            </Message>
            <DataTableFrame
              v-else
              :value="redemptions"
              :loading="redemptionsLoading"
              data-key="id"
              table-aria-label="奖励活动兑换记录"
              scrollable
              table-style="min-width: 940px"
              empty-title="暂无兑换记录"
            >
              <Column field="id" header="Redemption"
                ><template #body="{ data }">#{{ data.id }}</template></Column
              >
              <Column field="userId" header="用户"
                ><template #body="{ data }">User #{{ data.userId }}</template></Column
              >
              <Column header="Ideas"
                ><template #body="{ data }">{{
                  formatIdeaAmount(data.creditAmount)
                }}</template></Column
              >
              <Column field="maskedCodeHint" header="Code"
                ><template #body="{ data }"
                  ><span class="font-mono">{{ data.maskedCodeHint }}</span></template
                ></Column
              >
              <Column field="creditTransactionId" header="Credits Transaction"
                ><template #body="{ data }"
                  ><span class="block max-w-64 break-all font-mono text-admin-caption">{{
                    data.creditTransactionId
                  }}</span></template
                ></Column
              >
              <Column header="兑换时间"
                ><template #body="{ data }">{{
                  formatShanghaiDateTime(data.redeemedAt)
                }}</template></Column
              >
            </DataTableFrame>
          </AdminTablePanel>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <AdminActionReasonDialog
      v-model:visible="actionDialogVisible"
      :title="actionDialog.title"
      :message="actionDialog.message"
      :confirm-label="actionDialog.confirmLabel"
      :intent="actionDialog.intent"
      :loading="saving"
      @confirm="confirmAuditedAction"
    />
    <AdminActionReasonDialog
      v-model:visible="codeRevokeDialogVisible"
      title="确认撤销兑换码"
      :message="`将永久撤销 ${pendingCodeRevoke?.maskedCodeHint || '该兑换码'}，此操作不可恢复。`"
      confirm-label="确认撤销"
      intent="danger"
      :loading="codeRevokeLoading"
      @confirm="confirmCodeRevoke"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import { NavArrowLeft } from '@iconoir/vue'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import {
  ADMIN_REWARD_CAMPAIGNS_PATH,
  adminRewardCampaignDetailPath,
} from '../../../../shared/constants/routePaths'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import AdminActionReasonDialog from '../AdminActionReasonDialog.vue'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import RewardCampaignSettingsFields from './RewardCampaignSettingsFields.vue'
import RewardCodeBatchPanel from './RewardCodeBatchPanel.vue'
import {
  getRewardCampaignActionDescriptor,
  type RewardCampaignActionResult,
  type RewardCampaignAuditedAction,
} from './rewardCampaignActions'
import { createAdminRewardCodeResource, type AdminRewardCode } from './rewardCodeResource'
import { createRewardCampaignView } from './rewardCampaignPresentation'
import {
  createAdminRewardCampaignResource,
  type RewardCampaign,
  RewardCampaignConflictError,
  type UpdateRewardCampaignInput,
} from './rewardCampaignResource'
import {
  createAdminRewardRedemptionResource,
  type AdminRewardRedemption,
} from './rewardRedemptionResource'
import { formatShanghaiDateTime, isoToShanghaiDateTimeInput } from './rewardCampaignTime'

const route = useRoute()
const router = useRouter()
const campaignResource = createAdminRewardCampaignResource()
const codeResource = createAdminRewardCodeResource()
const redemptionResource = createAdminRewardRedemptionResource()
const toast = useAdminToast()
const adminConfirm = useAdminConfirm()
const campaignId = computed(() => Number(route.params.campaignId))
const campaign = ref<RewardCampaign | null>(null)
const activeTab = ref('overview')
const loading = ref(true)
const loadError = ref(false)
const saving = ref(false)
const publicCode = ref('')
const issuedPlaintext = ref('')
const pendingAction = ref<RewardCampaignAuditedAction | null>(null)
const actionDialogVisible = ref(false)
const editReasonError = ref('')
const edit = reactive<UpdateRewardCampaignInput>({
  displayName: '',
  internalNote: '',
  creditAmount: 0,
  maxRedemptions: 0,
  startsAt: '',
  expiresAt: '',
  reason: '',
})
const codeLookupInput = ref('')
const codeLookupLoading = ref(false)
const codeLookupError = ref('')
const locatedCode = ref<AdminRewardCode | null>(null)
const pendingCodeRevoke = ref<AdminRewardCode | null>(null)
const codeRevokeDialogVisible = ref(false)
const codeRevokeLoading = ref(false)
const redemptions = ref<AdminRewardRedemption[]>([])
const redemptionsLoading = ref(false)
const redemptionError = ref(false)
const redemptionPagination = useAdminCursorPagination<number>({ pending: redemptionsLoading })

const campaignView = computed(() =>
  campaign.value ? createRewardCampaignView(campaign.value) : null,
)
const editScheduleError = computed(() =>
  edit.startsAt && edit.expiresAt && edit.startsAt >= edit.expiresAt
    ? '结束时间必须晚于开始时间'
    : '',
)
const canUpdate = computed(() =>
  Boolean(
    campaign.value &&
    edit.displayName.trim() &&
    edit.creditAmount > 0 &&
    Number.isSafeInteger(edit.maxRedemptions) &&
    edit.maxRedemptions >= (campaign.value.mode === 'public' ? 1 : 0) &&
    !editScheduleError.value,
  ),
)
const actionDialog = computed(() =>
  getRewardCampaignActionDescriptor(pendingAction.value ?? 'activate').dialog(
    campaign.value?.displayName || '当前活动',
  ),
)
const overviewItems = computed(() =>
  campaignView.value
    ? [
        {
          label: '状态',
          value: `${campaignView.value.statusLabel} · ${campaignView.value.lifecycleLabel}`,
        },
        {
          label: '规则',
          value: `${campaignView.value.modeLabel} · 每次 ${formatIdeaAmount(campaignView.value.creditAmount)}`,
        },
        { label: '有效时间', value: campaignView.value.scheduleLabel },
        { label: '发行数量', value: `${campaignView.value.issuedCodeCount} 个` },
        { label: '已兑换', value: `${campaignView.value.redemptionCount} 次` },
        { label: '可用 Code', value: `${campaignView.value.validUnusedCodeCount} 个` },
        { label: '总 Capacity', value: campaignView.value.maxRedemptionsLabel },
        { label: '剩余 Capacity', value: remainingCapacity(campaignView.value) },
      ]
    : [],
)

onMounted(loadRouteCampaign)
watch(
  () => route.params.campaignId,
  async () => {
    campaign.value = null
    activeTab.value = 'overview'
    locatedCode.value = null
    redemptionPagination.reset()
    await loadRouteCampaign()
  },
)

async function loadRouteCampaign() {
  if (!Number.isSafeInteger(campaignId.value) || campaignId.value <= 0) {
    await router.replace(ADMIN_REWARD_CAMPAIGNS_PATH)
    return
  }
  await Promise.all([loadCampaign(), loadRedemptions()])
}

async function loadCampaign() {
  loading.value = true
  loadError.value = false
  try {
    const loaded = await campaignResource.get(campaignId.value)
    setCampaign(loaded)
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminDisplayError(error, '奖励活动详情加载失败'))
  } finally {
    loading.value = false
  }
}

function setCampaign(value: RewardCampaign) {
  campaign.value = value
  Object.assign(edit, {
    displayName: value.displayName,
    internalNote: value.internalNote,
    creditAmount: value.creditAmount,
    maxRedemptions: value.maxRedemptions,
    startsAt: isoToShanghaiDateTimeInput(value.startsAt),
    expiresAt: isoToShanghaiDateTimeInput(value.expiresAt),
    reason: '',
  })
}

function requestUpdate() {
  if (!campaign.value || !canUpdate.value) return
  const validation = validateAdminActionReason(edit.reason)
  if (!validation.valid) {
    editReasonError.value = validation.error
    return
  }
  adminConfirm.request({
    title: '确认更新奖励活动',
    message: `将更新“${campaign.value.displayName}”的活动设置，已发行兑换码的核心规则不可变。`,
    confirmLabel: '确认更新',
    intent: 'warn',
    onConfirm: async () => {
      await runCommand(
        async () => {
          setCampaign(
            await campaignResource.update(campaignId.value, {
              ...edit,
              reason: validation.normalizedReason,
            }),
          )
          toast.success('奖励活动已更新')
        },
        '更新奖励活动失败',
        'update',
      )
    },
  })
}

function requestAuditedAction(action: RewardCampaignAuditedAction) {
  if (!campaign.value || saving.value) return
  if (action === 'issue' && !publicCode.value.trim()) {
    toast.error('请输入公共兑换码')
    return
  }
  pendingAction.value = action
  actionDialogVisible.value = true
}

async function confirmAuditedAction(reason: string) {
  if (!campaign.value || !pendingAction.value) return
  const original = campaign.value
  const action = pendingAction.value
  const descriptor = getRewardCampaignActionDescriptor(action)
  const succeeded = await runCommand(
    async () => {
      const result = await descriptor.execute({
        resource: campaignResource,
        campaign: original,
        reason,
        publicCode: publicCode.value,
      })
      await applyAuditedActionResult(result)
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

async function applyAuditedActionResult(result: RewardCampaignActionResult) {
  if (result.type === 'campaign') setCampaign(result.campaign)
  else if (result.type === 'issued') {
    issuedPlaintext.value = result.plaintext
    await loadCampaign()
  } else await router.replace(ADMIN_REWARD_CAMPAIGNS_PATH)
}

async function lookupCode() {
  if (!codeLookupInput.value.trim() || codeLookupLoading.value) return
  codeLookupLoading.value = true
  codeLookupError.value = ''
  try {
    locatedCode.value = await codeResource.lookup(codeLookupInput.value)
    codeLookupInput.value = ''
  } catch (error) {
    locatedCode.value = null
    codeLookupError.value = formatAdminDisplayError(error, '查找兑换码失败')
  } finally {
    codeLookupLoading.value = false
  }
}

function requestCodeRevoke(code: AdminRewardCode) {
  pendingCodeRevoke.value = code
  codeRevokeDialogVisible.value = true
}
async function confirmCodeRevoke(reason: string) {
  if (!pendingCodeRevoke.value || codeRevokeLoading.value) return
  codeRevokeLoading.value = true
  try {
    locatedCode.value = await codeResource.revoke(pendingCodeRevoke.value.id, reason)
    codeRevokeDialogVisible.value = false
    pendingCodeRevoke.value = null
    toast.success('兑换码已撤销')
    if (locatedCode.value.campaignId === campaignId.value) await loadCampaign()
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '撤销兑换码失败'))
  } finally {
    codeRevokeLoading.value = false
  }
}

async function loadRedemptions() {
  redemptionsLoading.value = true
  redemptionError.value = false
  try {
    const page = await redemptionResource.list(campaignId.value, redemptionPagination.cursor.value)
    redemptions.value = page.redemptions
    redemptionPagination.setNextCursor(page.nextBeforeId)
  } catch {
    redemptionError.value = true
  } finally {
    redemptionsLoading.value = false
  }
}
function nextRedemptionPage() {
  if (redemptionPagination.advance()) void loadRedemptions()
}
function previousRedemptionPage() {
  if (redemptionPagination.retreat()) void loadRedemptions()
}
async function handleBatchChanged() {
  await loadCampaign()
}
function goBack() {
  void router.push(ADMIN_REWARD_CAMPAIGNS_PATH)
}
function openLocatedCampaign(targetCampaignId: number) {
  void router.push(adminRewardCampaignDetailPath(targetCampaignId))
}
function remainingCapacity(value: ReturnType<typeof createRewardCampaignView>) {
  return value.maxRedemptions > 0
    ? `${Math.max(0, value.maxRedemptions - value.redemptionCount)} 次`
    : `${value.validUnusedCodeCount} 个已发行 Code`
}
function codeStatusLabel(status: AdminRewardCode['status']) {
  return status === 'available' ? '可用' : status === 'redeemed' ? '已兑换' : '已撤销'
}
function codeStatusSeverity(status: AdminRewardCode['status']): 'success' | 'info' | 'secondary' {
  return status === 'available' ? 'success' : status === 'redeemed' ? 'info' : 'secondary'
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
    if (
      action === 'update' &&
      error instanceof RewardCampaignConflictError &&
      error.kind === 'rules_frozen'
    )
      toast.error('活动已发行兑换码，仅内部备注可以修改')
    else if (
      action === 'delete' &&
      error instanceof RewardCampaignConflictError &&
      error.kind === 'draft_not_empty'
    )
      toast.error('仅未发行兑换码的空 Draft 可以删除')
    else toast.error(formatAdminDisplayError(error, fallback))
    return false
  } finally {
    saving.value = false
  }
}
</script>
