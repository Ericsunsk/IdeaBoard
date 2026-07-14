<template>
  <AdminManagementFrame content-class="min-h-0 p-4 sm:p-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <AdminToolbarSearch v-model="searchKeyword" placeholder="搜索当前页 SKU" />
        </template>

        <template #metrics="{ metricClass }">
          <span :class="metricClass">{{ catalogItems.length }} 个 SKU / 当前页</span>
          <Tag :value="`${billableCount} 可计价`" severity="success" rounded />
          <Tag
            v-if="unpricedCount > 0"
            :value="`${unpricedCount} 未配置价格`"
            severity="warn"
            rounded
          />
        </template>

        <template #filters>
          <Select
            v-model="catalogFilter"
            :options="catalogFilterOptions"
            option-label="label"
            option-value="value"
            size="small"
            class="w-[190px]"
            aria-label="SKU 筛选"
          />
        </template>

        <template #actions="{ iconButtonClass }">
          <Button
            text
            :class="iconButtonClass"
            aria-label="刷新 SKU Catalog"
            :loading="catalogLoading"
            @click="loadCatalog()"
          >
            <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
          </Button>
          <Button
            label="新建 SKU"
            size="small"
            :class="adminActionButtonClass"
            @click="openCreateSKU"
          >
            <template #icon><Plus class="size-4" /></template>
          </Button>
        </template>

        <template #pager>
          <AdminPagerControls
            :current-page="currentCatalogPage"
            :total-pages="catalogKnownPageCount"
            :prev-disabled="currentCatalogPage === 1 || catalogLoading"
            :next-disabled="!catalogHasNext || catalogLoading"
            @prev="previousCatalogPage"
            @next="nextCatalogPage"
          />
        </template>
      </AdminResourceToolbar>
    </template>

    <Message v-if="catalogError" severity="error" :closable="false" class="mb-4">
      {{ catalogError }}。请刷新重试。
    </Message>

    <AdminBillingCoveragePanel
      :items="coverageItems"
      :loading="coverageLoading"
      :error="coverageError"
      @refresh="loadCoverage"
      @configure-sku="openCreateSKUForCoverage"
    />

    <div class="grid min-w-0 gap-4 xl:grid-cols-[minmax(460px,0.9fr)_minmax(560px,1.1fr)]">
      <section
        class="min-w-0 overflow-hidden rounded-[var(--radius-ib-panel)] border border-admin-border-soft bg-ib-surface"
        aria-labelledby="billing-sku-catalog-title"
      >
        <div
          class="flex min-h-14 items-center justify-between gap-3 border-b border-admin-border-soft px-4 py-3"
        >
          <div class="min-w-0">
            <h2
              id="billing-sku-catalog-title"
              class="m-0 text-admin-title-sm font-bold text-admin-title-color"
            >
              SKU Catalog
            </h2>
            <p class="m-0 mt-0.5 text-admin-caption text-admin-text-3">
              SKU identity 创建后永久占用，不可重命名或复用。
            </p>
          </div>
        </div>

        <DataTableFrame
          :value="visibleCatalogItems"
          :loading="catalogLoading"
          data-key="sku"
          scrollable
          frame-class="min-h-[420px]"
          :table-style="{ minWidth: '620px' }"
          selection-mode="single"
          :selection="selectedSKU"
          @row-select="selectSKU($event.data)"
        >
          <Column header="SKU" :style="{ minWidth: '280px' }">
            <template #body="{ data: item }">
              <div class="min-w-0">
                <span class="block truncate text-admin-body font-bold text-admin-text-1">
                  {{ item.displayName }}
                </span>
                <code
                  class="mt-0.5 block max-w-[320px] truncate text-admin-caption text-admin-text-3"
                  :title="item.sku"
                >
                  {{ item.sku }}
                </code>
              </div>
            </template>
          </Column>
          <Column header="当前价格" :style="{ width: '128px' }">
            <template #body="{ data: item }">
              <span
                v-if="item.currentPrice"
                class="font-mono text-admin-body font-bold text-admin-text-1"
              >
                {{ formatIdeaAmount(item.currentPrice.unitCredits) }}
              </span>
              <Tag v-else value="未配置" severity="warn" rounded :class="adminCompactTagClass" />
            </template>
          </Column>
          <Column header="状态" :style="{ width: '102px' }">
            <template #body="{ data: item }">
              <Tag
                :value="catalogStatus(item).label"
                :severity="catalogStatus(item).severity"
                rounded
                :class="adminCompactTagClass"
              />
            </template>
          </Column>
          <template #empty>
            <div :class="adminEmptyStateClass">
              <span class="font-semibold text-admin-title-color">当前页没有匹配的 SKU</span>
              <span class="mt-1 text-admin-meta">调整筛选，或创建第一个计费 SKU。</span>
            </div>
          </template>
        </DataTableFrame>
      </section>

      <section
        class="min-w-0 overflow-hidden rounded-[var(--radius-ib-panel)] border border-admin-border-soft bg-ib-surface"
        aria-labelledby="billing-price-history-title"
      >
        <template v-if="selectedSKU">
          <div
            class="flex flex-wrap items-start justify-between gap-3 border-b border-admin-border-soft px-4 py-3"
          >
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2
                  id="billing-price-history-title"
                  class="m-0 text-admin-title-sm font-bold text-admin-title-color"
                >
                  {{ selectedSKU.displayName }}
                </h2>
                <Tag
                  :value="selectedSKU.enabled ? 'SKU 已启用' : 'SKU 已停用'"
                  :severity="selectedSKU.enabled ? 'success' : 'secondary'"
                  rounded
                  :class="adminCompactTagClass"
                />
              </div>
              <code
                class="mt-1 block max-w-full truncate text-admin-caption text-admin-text-3"
                :title="selectedSKU.sku"
              >
                {{ selectedSKU.sku }}
              </code>
              <p v-if="selectedSKU.description" class="m-0 mt-1 text-admin-meta text-admin-text-2">
                {{ selectedSKU.description }}
              </p>
            </div>
            <div class="flex shrink-0 flex-wrap gap-2">
              <Button
                label="编辑 SKU"
                size="small"
                severity="secondary"
                outlined
                @click="openEditSKU"
              />
              <Button label="发布新价格" size="small" @click="openCreatePrice" />
            </div>
          </div>

          <div
            class="grid gap-3 border-b border-admin-border-soft bg-admin-surface-muted px-4 py-3 sm:grid-cols-3"
          >
            <div>
              <div class="text-admin-caption font-semibold text-admin-text-3">产品类型</div>
              <div class="mt-1 text-admin-body font-bold text-admin-text-1">
                {{
                  selectedAIWorkflow ? `AI 生图 · ${selectedAIWorkflow.feature.label}` : '通用 SKU'
                }}
              </div>
            </div>
            <div>
              <div class="text-admin-caption font-semibold text-admin-text-3">当前版本</div>
              <div class="mt-1 text-admin-body font-bold text-admin-text-1">
                {{ selectedSKU.currentPrice ? `v${selectedSKU.currentPrice.version}` : '未配置' }}
              </div>
            </div>
            <div>
              <div class="text-admin-caption font-semibold text-admin-text-3">计价状态</div>
              <div
                class="mt-1 text-admin-body font-bold"
                :class="isSelectedBillable ? 'text-admin-success' : 'text-admin-danger'"
              >
                {{ isSelectedBillable ? '可计价' : '不可计价' }}
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
            <div>
              <h3 class="m-0 text-admin-content font-bold text-admin-title-color">价格版本历史</h3>
              <p class="m-0 mt-0.5 text-admin-caption text-admin-text-3">
                价格数值不可修改；调整价格必须发布新 version。
              </p>
            </div>
            <div class="flex items-center gap-2">
              <Button
                text
                :class="adminFramedIconButtonClass"
                aria-label="刷新价格版本"
                :loading="pricesLoading"
                @click="loadPrices()"
              >
                <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
              </Button>
              <AdminPagerControls
                :current-page="currentPricePage"
                :total-pages="priceKnownPageCount"
                :prev-disabled="currentPricePage === 1 || pricesLoading"
                :next-disabled="!priceHasNext || pricesLoading"
                @prev="previousPricePage"
                @next="nextPricePage"
              />
            </div>
          </div>

          <Message v-if="pricesError" severity="error" :closable="false" class="!mx-4 !mb-3">
            {{ pricesError }}。请刷新重试。
          </Message>

          <DataTableFrame
            :value="prices"
            :loading="pricesLoading"
            data-key="priceVersionId"
            scrollable
            frame-class="min-h-[280px] border-t border-admin-border-soft"
            :table-style="{ minWidth: '650px' }"
          >
            <Column field="version" header="版本" :style="{ width: '90px' }">
              <template #body="{ data: price }">
                <div class="flex items-center gap-2">
                  <span class="font-mono font-bold">v{{ price.version }}</span>
                  <Tag
                    v-if="selectedSKU?.currentPriceVersionId === price.priceVersionId"
                    value="当前"
                    severity="info"
                    rounded
                    :class="adminCompactTagClass"
                  />
                </div>
              </template>
            </Column>
            <Column header="单价" :style="{ width: '140px' }">
              <template #body="{ data: price }">
                <span class="font-mono text-admin-body font-bold">{{
                  formatIdeaAmount(price.unitCredits)
                }}</span>
              </template>
            </Column>
            <Column header="发布信息" :style="{ minWidth: '180px' }">
              <template #body="{ data: price }">
                <div class="text-admin-body text-admin-text-1">Admin #{{ price.createdBy }}</div>
                <div class="mt-0.5 text-admin-caption text-admin-text-3">
                  {{ formatBillingDate(price.createdAt) }}
                </div>
              </template>
            </Column>
            <Column header="状态" :style="{ width: '128px' }">
              <template #body="{ data: price }">
                <div class="flex items-center gap-2">
                  <ToggleSwitch
                    :model-value="price.enabled"
                    :disabled="mutationPending"
                    :aria-label="`${price.enabled ? '停用' : '启用'}价格版本 v${price.version}`"
                    @update:model-value="requestPriceStateChange(price, $event)"
                  />
                  <span class="text-admin-meta text-admin-text-2">{{
                    price.enabled ? '启用' : '停用'
                  }}</span>
                </div>
              </template>
            </Column>
            <template #empty>
              <div :class="adminEmptyStateClass">
                <span class="font-semibold text-admin-title-color">尚未发布价格版本</span>
                <span class="mt-1 text-admin-meta"
                  >发布首个 version 后，该 SKU 才能参与 Quote。</span
                >
              </div>
            </template>
          </DataTableFrame>
        </template>

        <div v-else :class="adminEmptyStateClass">
          <span class="font-semibold text-admin-title-color">选择一个 SKU 查看价格版本</span>
          <span class="mt-1 text-admin-meta">右侧只展示服务端返回的真实 Catalog 数据。</span>
        </div>
      </section>
    </div>
  </AdminManagementFrame>

  <AdminDialogShell
    v-model:visible="showSKUDialog"
    :header="skuEditorMode === 'create' ? '创建 SKU' : '编辑 SKU'"
    :busy="mutationPending"
  >
    <Message v-if="skuEditorMode === 'create'" severity="warn" :closable="false">
      SKU 是全局 immutable identity。创建后不能删除、重命名或复用，请先确认 namespace 与 local key。
    </Message>
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="SKU"
      for="admin-billing-sku"
      :error="skuFormErrors.sku"
      :hint="
        skuEditorMode === 'create'
          ? '例如 ai-image:sketch-to-image.model-fingerprint'
          : 'Immutable identity'
      "
      required
    >
      <InputText
        id="admin-billing-sku"
        v-model="skuForm.sku"
        class="w-full font-mono"
        :disabled="skuEditorMode === 'edit'"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        autocomplete="off"
      />
    </AdminFormField>
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="展示名称"
      for="admin-billing-display-name"
      :error="skuFormErrors.displayName"
      required
    >
      <InputText
        id="admin-billing-display-name"
        v-model="skuForm.displayName"
        class="w-full"
        :invalid="invalid"
        :aria-describedby="describedbyId"
      />
    </AdminFormField>
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="说明"
      for="admin-billing-description"
      :error="skuFormErrors.description"
    >
      <Textarea
        id="admin-billing-description"
        v-model="skuForm.description"
        class="w-full"
        rows="4"
        :invalid="invalid"
        :aria-describedby="describedbyId"
      />
    </AdminFormField>
    <AdminFormField label="SKU 状态" for="admin-billing-sku-enabled">
      <div
        class="flex min-h-10 items-center justify-between rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface px-3"
      >
        <span class="text-admin-body font-medium text-admin-text-1">{{
          skuForm.enabled ? '启用' : '停用'
        }}</span>
        <ToggleSwitch
          v-model="skuForm.enabled"
          input-id="admin-billing-sku-enabled"
          aria-label="SKU 启用状态"
        />
      </div>
    </AdminFormField>
    <AdminActionReasonField
      v-model="skuForm.reason"
      input-id="admin-billing-sku-reason"
      :error="skuFormErrors.reason"
      @update:model-value="skuFormErrors.reason = undefined"
    />
    <template #footer>
      <Button
        label="取消"
        severity="secondary"
        outlined
        :disabled="mutationPending"
        @click="showSKUDialog = false"
      />
      <Button label="确认并保存" :loading="mutationPending" @click="requestSaveSKU" />
    </template>
  </AdminDialogShell>

  <AdminDialogShell
    v-model:visible="showPriceDialog"
    header="发布新价格版本"
    :busy="mutationPending"
  >
    <Message severity="info" :closable="false">
      {{ selectedSKU?.sku }} 的历史价格不可修改。新 version 发布后会成为当前价格快照。
    </Message>
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="单价（Idea）"
      for="admin-billing-unit-credits"
      :error="priceFormErrors.unitCredits"
      hint="0 表示免费，但仍会记录 Billing snapshot。"
      required
    >
      <InputNumber
        v-model="priceForm.unitCredits"
        input-id="admin-billing-unit-credits"
        fluid
        :min="0"
        :max="9000000000000000"
        :use-grouping="true"
        :invalid="invalid"
        :aria-describedby="describedbyId"
      />
    </AdminFormField>
    <AdminFormField label="发布状态" for="admin-billing-price-enabled">
      <div
        class="flex min-h-10 items-center justify-between rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface px-3"
      >
        <span class="text-admin-body font-medium text-admin-text-1">{{
          priceForm.enabled ? '发布后立即启用' : '发布为停用'
        }}</span>
        <ToggleSwitch
          v-model="priceForm.enabled"
          input-id="admin-billing-price-enabled"
          aria-label="新价格版本启用状态"
        />
      </div>
    </AdminFormField>
    <AdminActionReasonField
      v-model="priceForm.reason"
      input-id="admin-billing-price-reason"
      :error="priceFormErrors.reason"
      hint="将进入不可变审计记录，请说明调价依据。"
      @update:model-value="priceFormErrors.reason = undefined"
    />
    <template #footer>
      <Button
        label="取消"
        severity="secondary"
        outlined
        :disabled="mutationPending"
        @click="showPriceDialog = false"
      />
      <Button label="确认发布" :loading="mutationPending" @click="requestCreatePrice" />
    </template>
  </AdminDialogShell>

  <AdminActionReasonDialog
    v-model:visible="showPriceStateReasonDialog"
    :title="priceStateTarget?.enabled ? '确认启用价格版本' : '确认停用价格版本'"
    :message="priceStateReasonMessage"
    :confirm-label="priceStateTarget?.enabled ? '确认启用' : '确认停用'"
    :intent="priceStateTarget?.enabled ? 'warn' : 'danger'"
    :loading="mutationPending"
    @confirm="confirmPriceStateChange"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { Plus, Refresh } from '@iconoir/vue'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../../shared/domain/workflowFeatureRegistry'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import { useAdminFeedback } from '../../feedback/useAdminFeedback'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import {
  adminActionButtonClass,
  adminCompactTagClass,
  adminEmptyStateClass,
  adminFramedIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import AdminActionReasonDialog from '../AdminActionReasonDialog.vue'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import {
  formatBillingDate,
  isCatalogItemBillable,
  parseAIWorkflowSKU,
  validateBillingPriceForm,
  validateBillingSKUForm,
} from './billingPricingModel'
import type {
  AdminBillingCatalogItem,
  AdminBillingCoverageItem,
  AdminBillingPrice,
  BillingCatalogFilter,
  BillingPriceForm,
  BillingPriceFormErrors,
  BillingSKUForm,
  BillingSKUFormErrors,
} from './billingPricingTypes'
import AdminBillingCoveragePanel from './AdminBillingCoveragePanel.vue'
import { useAdminBillingPricing } from './useAdminBillingPricing'

const adminFeedback = useAdminFeedback()
const adminConfirm = useAdminConfirm()
const pricing = useAdminBillingPricing({
  feedback: adminFeedback.creditsBilling.pricing,
})

const {
  coverageItems,
  catalogItems,
  visibleCatalogItems,
  selectedSKU,
  prices,
  searchKeyword,
  catalogFilter,
  coverageLoading,
  catalogLoading,
  pricesLoading,
  coverageError,
  catalogError,
  pricesError,
  mutationPending,
  currentCatalogPage,
  catalogKnownPageCount,
  catalogHasNext,
  currentPricePage,
  priceKnownPageCount,
  priceHasNext,
  unpricedCount,
  billableCount,
  loadCoverage,
  loadCatalog,
  loadPrices,
  selectSKU,
  nextCatalogPage,
  previousCatalogPage,
  nextPricePage,
  previousPricePage,
} = pricing

const showSKUDialog = ref(false)
const showPriceDialog = ref(false)
const showPriceStateReasonDialog = ref(false)
const priceStateTarget = ref<{ price: AdminBillingPrice; enabled: boolean }>()
const skuEditorMode = ref<'create' | 'edit'>('create')
const skuForm = reactive<BillingSKUForm>(emptySKUForm())
const priceForm = reactive<BillingPriceForm>({ unitCredits: null, enabled: true, reason: '' })
const skuFormErrors = reactive<BillingSKUFormErrors>({})
const priceFormErrors = reactive<BillingPriceFormErrors>({})
const selectedAIWorkflow = computed(() =>
  selectedSKU.value ? parseAIWorkflowSKU(selectedSKU.value.sku) : undefined,
)
const isSelectedBillable = computed(() =>
  selectedSKU.value ? isCatalogItemBillable(selectedSKU.value) : false,
)
const priceStateReasonMessage = computed(() => {
  const target = priceStateTarget.value
  if (!target) return '请填写操作原因后再确认。'
  const skuLabel = selectedSKU.value?.displayName || selectedSKU.value?.sku || '当前 SKU'
  return `${target.enabled ? '启用' : '停用'} ${skuLabel} 的 v${target.price.version}（${formatIdeaAmount(target.price.unitCredits)}）。`
})
const catalogFilterOptions: { label: string; value: BillingCatalogFilter }[] = [
  { label: '全部 SKU（当前页）', value: 'all' },
  { label: 'AI 生图 SKU', value: 'ai-image' },
  { label: '未配置价格（当前页）', value: 'unpriced' },
  { label: '已停用 SKU', value: 'disabled' },
]

onMounted(() => {
  void Promise.all([loadCatalog(), loadCoverage()])
})

function openCreateSKU() {
  skuEditorMode.value = 'create'
  Object.assign(skuForm, emptySKUForm())
  clearObject(skuFormErrors)
  showSKUDialog.value = true
}

function openCreateSKUForCoverage(item: AdminBillingCoverageItem) {
  skuEditorMode.value = 'create'
  Object.assign(skuForm, {
    sku: item.sku,
    displayName: `${coverageFeatureLabel(item)} / ${item.model}`,
    description: `AI 生图计费：${item.feature} × ${item.model}`,
    enabled: true,
    reason: '',
  })
  clearObject(skuFormErrors)
  showSKUDialog.value = true
}

function coverageFeatureLabel(item: AdminBillingCoverageItem) {
  return WORKFLOW_FEATURE_REGISTRY_MAP[item.feature].label
}

function openEditSKU() {
  if (!selectedSKU.value) return
  skuEditorMode.value = 'edit'
  Object.assign(skuForm, {
    sku: selectedSKU.value.sku,
    displayName: selectedSKU.value.displayName,
    description: selectedSKU.value.description,
    enabled: selectedSKU.value.enabled,
    reason: '',
  })
  clearObject(skuFormErrors)
  showSKUDialog.value = true
}

function openCreatePrice() {
  priceForm.unitCredits = selectedSKU.value?.currentPrice?.unitCredits ?? null
  priceForm.enabled = true
  priceForm.reason = ''
  clearObject(priceFormErrors)
  showPriceDialog.value = true
}

function requestSaveSKU() {
  clearObject(skuFormErrors)
  Object.assign(skuFormErrors, validateBillingSKUForm(skuForm, skuEditorMode.value === 'edit'))
  if (Object.keys(skuFormErrors).length > 0) return

  const isCreate = skuEditorMode.value === 'create'
  adminConfirm.request({
    title: isCreate ? '确认创建 immutable SKU' : '确认更新 SKU',
    message: isCreate
      ? `SKU ${skuForm.sku.trim().toLocaleLowerCase()} 创建后不能重命名、删除或复用。`
      : `将更新 ${skuForm.sku} 的展示信息与启用状态。`,
    confirmLabel: isCreate ? '确认创建' : '确认更新',
    intent: 'warn',
    onConfirm: async () => {
      const succeeded = isCreate
        ? await pricing.createSKU({
            sku: skuForm.sku,
            displayName: skuForm.displayName,
            description: skuForm.description,
            enabled: skuForm.enabled,
            reason: skuForm.reason,
          })
        : await pricing.updateSKU({
            displayName: skuForm.displayName,
            description: skuForm.description,
            enabled: skuForm.enabled,
            reason: skuForm.reason,
          })
      if (succeeded) showSKUDialog.value = false
    },
  })
}

function requestCreatePrice() {
  clearObject(priceFormErrors)
  Object.assign(priceFormErrors, validateBillingPriceForm(priceForm))
  if (Object.keys(priceFormErrors).length > 0 || priceForm.unitCredits == null) return

  const unitCredits = priceForm.unitCredits
  adminConfirm.request({
    title: '确认发布新价格版本',
    message: `将为 ${selectedSKU.value?.sku ?? '当前 SKU'} 发布 ${formatIdeaAmount(unitCredits)} 的 immutable price version${priceForm.enabled ? '，并立即启用' : ''}。`,
    confirmLabel: '确认发布',
    intent: 'warn',
    onConfirm: async () => {
      const succeeded = await pricing.createPrice({
        unitCredits,
        enabled: priceForm.enabled,
        reason: priceForm.reason,
      })
      if (succeeded) showPriceDialog.value = false
    },
  })
}

function requestPriceStateChange(price: AdminBillingPrice, enabled: boolean) {
  priceStateTarget.value = { price, enabled }
  showPriceStateReasonDialog.value = true
}

async function confirmPriceStateChange(reason: string) {
  const target = priceStateTarget.value
  if (!target) return
  const succeeded = await pricing.setPriceEnabled(target.price, target.enabled, reason)
  if (!succeeded) return
  showPriceStateReasonDialog.value = false
  priceStateTarget.value = undefined
}

function catalogStatus(item: AdminBillingCatalogItem) {
  if (!item.enabled) return { label: 'SKU 停用', severity: 'secondary' as const }
  if (!item.currentPrice) return { label: '未配置', severity: 'warn' as const }
  if (!item.currentPrice.enabled) return { label: '价格停用', severity: 'danger' as const }
  return { label: '可计价', severity: 'success' as const }
}

function emptySKUForm(): BillingSKUForm {
  return { sku: '', displayName: '', description: '', enabled: true, reason: '' }
}

function clearObject(target: object) {
  for (const key of Object.keys(target)) {
    delete (target as Record<string, unknown>)[key]
  }
}
</script>
