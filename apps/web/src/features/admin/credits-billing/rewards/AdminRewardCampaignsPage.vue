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
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch
              v-model="searchInput"
              placeholder="搜索活动名称"
              @enter="applySearch"
              @clear="applySearch"
            />
          </template>
          <template #filters>
            <Select
              v-model="modeFilter"
              :options="modeFilterOptions"
              option-label="label"
              option-value="value"
              size="small"
              class="w-32"
              aria-label="Code 模式筛选"
              @change="applyFilters"
            />
            <Select
              v-model="statusFilter"
              :options="statusFilterOptions"
              option-label="label"
              option-value="value"
              size="small"
              class="w-32"
              aria-label="活动状态筛选"
              @change="applyFilters"
            />
          </template>
          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新奖励活动"
              :loading="loading"
              @click="loadCampaigns"
            >
              <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
            </Button>
          </template>
          <template #pager>
            <AdminPagerControls
              :current-page="pagination.currentPage.value"
              :total-pages="pagination.knownPageCount.value"
              :prev-disabled="!pagination.canRetreat.value"
              :next-disabled="!pagination.canAdvance.value"
              @prev="previousPage"
              @next="nextPage"
            />
          </template>
        </AdminResourceToolbar>
      </template>

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
        table-style="min-width: 1120px"
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
        <Column header="Code 模式"
          ><template #body="{ data }">{{ data.modeLabel }}</template></Column
        >
        <Column header="有效时间">
          <template #body="{ data }"
            ><span class="whitespace-nowrap text-admin-meta text-admin-text-2">{{
              data.scheduleLabel
            }}</span></template
          >
        </Column>
        <Column header="单次奖励"
          ><template #body="{ data }">{{ formatIdeaAmount(data.creditAmount) }}</template></Column
        >
        <Column header="Code">
          <template #body="{ data }">
            <div class="grid min-w-24 gap-0.5 text-admin-meta">
              <span>{{ data.issuedCodeCount }} 已发行</span>
              <span class="text-admin-text-3">{{ data.validUnusedCodeCount }} 可用</span>
            </div>
          </template>
        </Column>
        <Column header="兑换进度"
          ><template #body="{ data }">{{ data.progressLabel }}</template></Column
        >
        <Column header="操作">
          <template #body="{ data }">
            <Button size="small" label="管理" text @click="openCampaign(data.id)" />
          </template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import { Refresh } from '@iconoir/vue'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { adminRewardCampaignDetailPath } from '../../../../shared/constants/routePaths'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import RewardCampaignSettingsFields from './RewardCampaignSettingsFields.vue'
import { createRewardCampaignView } from './rewardCampaignPresentation'
import {
  createAdminRewardCampaignResource,
  type CreateRewardCampaignInput,
  type RewardCampaign,
} from './rewardCampaignResource'
import { shanghaiDateTimeInputToISO } from './rewardCampaignTime'

const router = useRouter()
const resource = createAdminRewardCampaignResource()
const toast = useAdminToast()
const campaigns = ref<RewardCampaign[]>([])
const loading = ref(true)
const loadError = ref(false)
const saving = ref(false)
const searchInput = ref('')
const appliedSearch = ref('')
const modeFilter = ref<'' | RewardCampaign['mode']>('')
const statusFilter = ref<'' | RewardCampaign['status']>('')
const pagination = useAdminCursorPagination<number>({ pending: loading })

const campaignModeOptions = [
  { label: '公共码', value: 'public' },
  { label: '单次码', value: 'single_use' },
] satisfies Array<{ label: string; value: CreateRewardCampaignInput['mode'] }>
const modeFilterOptions = [{ label: '全部模式', value: '' }, ...campaignModeOptions]
const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '草稿', value: 'draft' },
  { label: '进行中', value: 'active' },
  { label: '待开始', value: 'scheduled' },
  { label: '已暂停', value: 'paused' },
  { label: '已耗尽', value: 'exhausted' },
  { label: '已过期', value: 'expired' },
  { label: '已取消', value: 'cancelled' },
] satisfies Array<{ label: string; value: '' | RewardCampaign['status'] }>
const draft = reactive<CreateRewardCampaignInput>(emptyCampaignForm())
const campaignViews = computed(() => campaigns.value.map(createRewardCampaignView))
const createScheduleError = computed(() => scheduleError(draft.startsAt, draft.expiresAt))
const canCreate = computed(
  () =>
    Boolean(draft.displayName.trim()) &&
    draft.creditAmount > 0 &&
    validRedemptionCap(draft.mode, draft.maxRedemptions) &&
    validateAdminActionReason(draft.reason).valid &&
    !createScheduleError.value,
)

onMounted(loadCampaigns)

async function loadCampaigns() {
  loading.value = true
  loadError.value = false
  try {
    const page = await resource.list(pagination.cursor.value, {
      search: appliedSearch.value || undefined,
      mode: modeFilter.value || undefined,
      status: statusFilter.value || undefined,
    })
    campaigns.value = page.campaigns
    pagination.setNextCursor(page.nextBeforeId)
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminDisplayError(error, '奖励活动加载失败'))
  } finally {
    loading.value = false
  }
}

function applySearch() {
  appliedSearch.value = searchInput.value.trim()
  applyFilters()
}

function applyFilters() {
  pagination.reset()
  void loadCampaigns()
}

function nextPage() {
  if (pagination.advance()) void loadCampaigns()
}

function previousPage() {
  if (pagination.retreat()) void loadCampaigns()
}

async function createCampaign() {
  if (!canCreate.value || saving.value) return
  saving.value = true
  try {
    const campaign = await resource.create({ ...draft })
    toast.success('奖励活动已创建')
    Object.assign(draft, emptyCampaignForm())
    pagination.reset()
    await loadCampaigns()
    await router.push(adminRewardCampaignDetailPath(campaign.id))
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '创建奖励活动失败'))
  } finally {
    saving.value = false
  }
}

function openCampaign(campaignId: number) {
  void router.push(adminRewardCampaignDetailPath(campaignId))
}

function emptyCampaignForm(): CreateRewardCampaignInput {
  return {
    displayName: '',
    internalNote: '',
    creditAmount: 100,
    mode: 'public',
    maxRedemptions: 100,
    startsAt: '',
    expiresAt: '',
    reason: '',
  }
}

function validRedemptionCap(mode: RewardCampaign['mode'], value: number) {
  return Number.isSafeInteger(value) && (mode === 'public' ? value > 0 : value >= 0)
}

function scheduleError(startsAt: string, expiresAt: string) {
  const start = shanghaiDateTimeInputToISO(startsAt)
  const end = shanghaiDateTimeInputToISO(expiresAt)
  return start && end && Date.parse(start) >= Date.parse(end) ? '结束时间必须晚于开始时间' : ''
}
</script>
