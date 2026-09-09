<template>
  <div>
    <AdminResourceGridFrame
      empty-description=""
      empty-title=""
      :is-empty="providerEndpointCards.length === 0"
      :is-loading="isProviderEndpointLoading"
    >
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch v-model="searchKeyword" input-label="搜索供应商端点" />
          </template>

          <template #metrics="{ metricClass }">
            <span :class="metricClass">
              <span class="h-2.5 w-2.5 shrink-0 rounded-full bg-admin-success"></span>
              <span>{{ totalProviderEndpointsLabel }}</span>
            </span>
            <Tag :value="accountRunningLabel" severity="success" rounded />
            <Tag :value="`${activeModelCount} 模型`" severity="info" rounded />
          </template>

          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新供应商端点"
              @click="commands.loadData"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
            <Button
              label="新增端点"
              :class="adminActionButtonClass"
              @click="providerEndpointEditor.openCreate"
            >
              <template #icon>
                <Plus class="size-4" />
              </template>
            </Button>
          </template>
        </AdminResourceToolbar>
      </template>

      <template #prepend>
        <SearchProviderCard />
      </template>

      <AdminProviderEndpointCard
        v-for="card in providerEndpointCards"
        :key="card.providerEndpoint.id"
        :card="card"
        :commands="commands"
        :provider-endpoint-editor="providerEndpointEditor"
        :account-editor="accountEditor"
        :active-logo-target-id="activeLogoTarget?.id"
        @open-logo-picker="openLogoPicker"
      />
    </AdminResourceGridFrame>

    <!-- 供应商端点表单弹窗 -->
    <AdminProviderEndpointDialog />

    <!-- APIKey 配置弹窗 -->
    <AdminProviderAccountDialog />

    <AdminLogoPickerPopover
      ref="logoPickerPopoverRef"
      scope="provider-endpoint"
      :target-id="activeLogoTarget?.id"
      :target-name="activeLogoTarget?.name"
      :target-base-url="activeLogoTarget?.baseUrl"
      :target-api-type="activeLogoTarget?.apiType"
      @hide="activeLogoTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { Plus, Refresh } from '@iconoir/vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminActionButtonClass } from '../../shared/lib/adminUiClasses'
import {
  provideAdminProviderConfig,
  useAdminProviderConfig,
} from '../useAdminProviderConfig'
import { useAdminProviderConfigFeedback } from '../useAdminProviderConfigFeedback'
import SearchProviderCard from '../SearchProviderCard.vue'
import AdminLogoPickerPopover from '../AdminLogoPickerPopover.vue'
import AdminProviderEndpointCard from '../AdminProviderEndpointCard.vue'
import AdminProviderEndpointDialog from '../AdminProviderEndpointDialog.vue'
import AdminProviderAccountDialog from '../AdminProviderAccountDialog.vue'
import type { ProviderEndpointCard } from '../providerConfigTypes'

const feedback = useAdminProviderConfigFeedback()
const adminConfirm = useAdminConfirm()

const providerConfig = useAdminProviderConfig({
  feedback,
  confirm: adminConfirm,
})
provideAdminProviderConfig(providerConfig)

const { cards, providerEndpointEditor, accountEditor, commands } = providerConfig

const activeLogoTarget = ref<{
  id: number
  name: string
  baseUrl: string
  apiType: string
} | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function openLogoPicker(event: Event, card: ProviderEndpointCard) {
  activeLogoTarget.value = {
    id: card.providerEndpoint.id,
    name: card.providerEndpoint.name,
    baseUrl: card.providerEndpoint.baseUrl,
    apiType: card.providerEndpoint.apiType,
  }
  logoPickerPopoverRef.value?.toggle(event)
}

const searchKeyword = cards.searchKeyword
const providerEndpointCards = cards.items
const isProviderEndpointLoading = cards.isLoading
const totalProviderEndpointsLabel = cards.totalProviderEndpointsLabel
const accountRunningLabel = cards.accountRunningLabel
const activeModelCount = cards.activeModelCount

onMounted(() => {
  commands.loadData()
})
</script>
