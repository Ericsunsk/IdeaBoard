<template>
  <div class="flex min-w-0 flex-col gap-4">
    <section class="border-b border-admin-border-soft bg-ib-surface px-1 pb-5">
      <div class="mb-4">
        <h2 class="m-0 text-admin-title font-[760] text-admin-title-color">奖励活动</h2>
      </div>
      <form class="grid gap-3 lg:grid-cols-2" @submit.prevent="createCampaign">
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          展示名称
          <InputText v-model="draft.displayName" unstyled :class="ibFormInputClass" />
        </label>
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          内部备注
          <InputText v-model="draft.internalNote" unstyled :class="ibFormInputClass" />
        </label>
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          Idea 数量
          <InputNumber v-model="draft.creditAmount" :min="1" :use-grouping="false" />
        </label>
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          总兑换上限
          <InputNumber v-model="draft.maxRedemptions" :min="1" :use-grouping="false" />
        </label>
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2 lg:col-span-2">
          操作原因
          <InputText v-model="draft.reason" unstyled :class="ibFormInputClass" />
        </label>
        <div class="lg:col-span-2">
          <Button type="submit" label="创建公共 Draft" :loading="saving" :disabled="!canCreate" />
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
        :value="campaigns"
        :loading="loading"
        data-key="id"
        table-aria-label="奖励活动列表"
        empty-title="暂无奖励活动"
      >
        <Column field="displayName" header="活动" />
        <Column header="状态">
          <template #body="{ data }">{{ data.status === 'active' ? '进行中' : 'Draft' }}</template>
        </Column>
        <Column header="奖励"
          ><template #body="{ data }">{{ data.creditAmount }} Ideas</template></Column
        >
        <Column header="进度">
          <template #body="{ data }"
            >{{ data.redemptionCount }} / {{ data.maxRedemptions }}</template
          >
        </Column>
        <Column header="操作">
          <template #body="{ data }">
            <Button size="small" label="管理" text @click="selectCampaign(data)" />
          </template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>

    <section v-if="selected" class="border-t border-admin-border-soft bg-ib-surface px-1 pt-5">
      <h3 class="mt-0 text-admin-title-sm font-[740] text-admin-title-color">
        {{ selected.displayName }}
      </h3>
      <div v-if="selected.status === 'draft'" class="grid gap-3 lg:grid-cols-2">
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          公共兑换码
          <InputText v-model="publicCode" unstyled autocomplete="off" :class="ibFormInputClass" />
        </label>
        <label class="grid gap-1 text-admin-body font-[650] text-admin-text-2">
          发行原因
          <InputText v-model="issueReason" unstyled :class="ibFormInputClass" />
        </label>
        <div class="flex flex-wrap gap-2 lg:col-span-2">
          <Button label="发行公共兑换码" :loading="saving" @click="issueCode" />
          <InputText
            v-model="activateReason"
            unstyled
            placeholder="激活原因"
            :class="ibFormInputClass"
          />
          <Button
            label="激活活动"
            severity="secondary"
            :loading="saving"
            @click="activateCampaign"
          />
        </div>
      </div>
      <Message v-if="issuedPlaintext" severity="success" :closable="false" class="mt-3">
        兑换码仅本次显示：<strong>{{ issuedPlaintext }}</strong>
      </Message>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { ibFormInputClass } from '../../../../shared/theme/formControls'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminToast, formatAdminApiError } from '../../shared/lib/adminToast'
import { createAdminRewardCampaignResource, type RewardCampaign } from './rewardCampaignResource'

const resource = createAdminRewardCampaignResource()
const toast = useAdminToast()
const campaigns = ref<RewardCampaign[]>([])
const selected = ref<RewardCampaign | null>(null)
const loading = ref(true)
const loadError = ref(false)
const saving = ref(false)
const issuedPlaintext = ref('')
const publicCode = ref('')
const issueReason = ref('')
const activateReason = ref('')
const draft = reactive({
  displayName: '',
  internalNote: '',
  creditAmount: 0,
  maxRedemptions: 0,
  reason: '',
})
const canCreate = computed(() =>
  Boolean(
    draft.displayName.trim() &&
    draft.reason.trim() &&
    draft.creditAmount > 0 &&
    draft.maxRedemptions > 0,
  ),
)

onMounted(loadCampaigns)

async function loadCampaigns() {
  loading.value = true
  loadError.value = false
  try {
    campaigns.value = (await resource.list()).campaigns
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminApiError(error, '加载奖励活动失败'))
  } finally {
    loading.value = false
  }
}

async function createCampaign() {
  if (!canCreate.value) return
  await runCommand(async () => {
    const campaign = await resource.create(draft)
    campaigns.value.unshift(campaign)
    selected.value = campaign
    toast.success('奖励活动已创建')
  })
}

function selectCampaign(campaign: RewardCampaign) {
  selected.value = campaign
  issuedPlaintext.value = ''
}

async function issueCode() {
  if (!selected.value || !publicCode.value.trim() || !issueReason.value.trim()) return
  await runCommand(async () => {
    const issued = await resource.issuePublicCode(selected.value!.id, {
      code: publicCode.value,
      reason: issueReason.value,
    })
    issuedPlaintext.value = issued.plaintext
    toast.success('公共兑换码已发行')
  })
}

async function activateCampaign() {
  if (!selected.value || !activateReason.value.trim()) return
  await runCommand(async () => {
    const activated = await resource.activate(selected.value!.id, activateReason.value)
    selected.value = activated
    const index = campaigns.value.findIndex((campaign) => campaign.id === activated.id)
    if (index >= 0) campaigns.value[index] = activated
    toast.success('奖励活动已激活')
  })
}

async function runCommand(command: () => Promise<void>) {
  if (saving.value) return
  saving.value = true
  try {
    await command()
  } catch (error) {
    toast.error(formatAdminApiError(error, '奖励操作失败'))
  } finally {
    saving.value = false
  }
}
</script>
