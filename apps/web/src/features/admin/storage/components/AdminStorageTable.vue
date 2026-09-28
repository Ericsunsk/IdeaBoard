<template>
  <!-- 加载中状态 (AdminSpinner 自带 role="progressbar") -->
  <div
    v-if="loading && rows.length === 0"
    class="flex min-h-[260px] items-center justify-center rounded-xl border border-admin-border-soft bg-white text-admin-text-3"
    :aria-label="STORAGE_MESSAGES.tableLoadingAria"
  >
    <AdminSpinner class="size-9 text-admin-accent" />
  </div>

  <!-- 空状态提示 (无数据或过滤未命中) -->
  <div
    v-else-if="rows.length === 0"
    class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-admin-border-soft bg-white p-10 text-center"
  >
    <div
      class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-admin-surface-muted text-admin-text-3"
    >
      <DatabaseExport v-if="!hasActiveFilter" class="size-6" />
      <Search v-else class="size-6" />
    </div>
    <h3 class="text-base font-semibold text-admin-title-color">
      {{ STORAGE_MESSAGES.tableEmptyTitle(hasActiveFilter) }}
    </h3>
    <p class="mt-1 max-w-sm text-xs text-admin-text-3">
      {{ STORAGE_MESSAGES.tableEmptyHint(hasActiveFilter) }}
    </p>
    <button
      v-if="hasActiveFilter"
      type="button"
      class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
      @click="emit('clear-filters')"
    >
      <Xmark class="size-3.5" />
      <span>{{ STORAGE_MESSAGES.tableClearFilters }}</span>
    </button>
  </div>

  <!-- 存储配置表格 (1:1 对齐功能定价页 BillingPricingTable 规范，预设厂商平铺展示) -->
  <section v-else :aria-label="STORAGE_MESSAGES.tableSectionAria">
    <div class="mb-2 flex items-center gap-2">
      <h3 class="text-sm font-bold text-admin-title-color">
        {{ STORAGE_MESSAGES.tableSectionAria }}
      </h3>
      <span class="text-xs text-admin-text-3">{{
        STORAGE_MESSAGES.tableSectionCount(rows.length)
      }}</span>
    </div>

    <div :class="adminTableCardClass">
      <AdminTable
        :value="rows"
        :loading="loading"
        data-key="id"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
        :table-aria-label="STORAGE_MESSAGES.tableAria"
        :scroll-aria-label="STORAGE_MESSAGES.tableScrollAria"
      >
        <template #empty>
          <AdminEmptyState :title="STORAGE_MESSAGES.tableEmptyMatch" />
        </template>

        <!-- 存储服务列 (图标/Logo + 名称 + 供应商标签) -->
        <AdminColumn
          field="service"
          :header="STORAGE_MESSAGES.serviceColumn"
          :style="{ width: '240px', minWidth: '200px' }"
        >
          <template #body="{ data }">
            <div class="flex min-w-0 items-center gap-2.5">
              <button
                v-if="!data.isPresetOnly && data.storage"
                type="button"
                :class="[adminLogoButtonClass, 'size-8 p-1.5 shrink-0 rounded-lg shadow-xs']"
                :data-control-state="activeLogoTargetId === data.storage.id ? 'selected' : 'idle'"
                :aria-expanded="activeLogoTargetId === data.storage.id ? 'true' : 'false'"
                :title="STORAGE_MESSAGES.switchLogoTitle"
                :aria-label="STORAGE_MESSAGES.switchLogoAria"
                @click="emit('open-logo-picker', $event, data)"
              >
                <img
                  v-if="data.vendorLogo"
                  :src="data.vendorLogo"
                  :alt="data.name"
                  class="size-full object-contain"
                />
                <component
                  :is="data.vendorIcon"
                  v-else-if="data.vendorIcon"
                  class="size-4 text-admin-text-2"
                />
                <span
                  v-else
                  class="text-admin-caption font-bold text-admin-accent uppercase select-none"
                >
                  {{ data.avatarLabel }}
                </span>
              </button>
              <div
                v-else
                class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-white p-1.5 shadow-xs"
              >
                <img
                  v-if="data.vendorLogo"
                  :src="data.vendorLogo"
                  :alt="data.name"
                  class="size-full object-contain opacity-80"
                />
                <component
                  :is="data.vendorIcon"
                  v-else-if="data.vendorIcon"
                  class="size-4 text-admin-text-3"
                />
                <span
                  v-else
                  class="text-admin-caption font-bold text-admin-text-3 uppercase select-none"
                >
                  {{ data.avatarLabel }}
                </span>
              </div>
              <div class="flex min-w-0 flex-col">
                <span
                  class="truncate text-xs font-bold"
                  :class="data.isPresetOnly ? 'text-admin-text-2' : 'text-admin-title-color'"
                >
                  {{ data.name }}
                </span>
                <span class="truncate text-admin-caption font-medium text-admin-text-3">{{
                  data.subtitle
                }}</span>
              </div>
            </div>
          </template>
        </AdminColumn>

        <!-- 状态列 (对齐 BillingStatusCell 胶囊 + 圆点规范) -->
        <AdminColumn
          field="status"
          :header="STORAGE_MESSAGES.statusColumn"
          :style="{ width: '110px' }"
        >
          <template #body="{ data }">
            <span
              v-if="data.isCurrent"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-medium leading-none bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200"
            >
              <span class="size-1.5 rounded-full bg-emerald-500" />
              <span>{{ STORAGE_MESSAGES.current }}</span>
            </span>
            <span
              v-else-if="data.statusType === 'configured'"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-medium leading-none bg-sky-50 text-sky-700 ring-1 ring-sky-200"
            >
              <span class="size-1.5 rounded-full bg-sky-500" />
              <span>{{ STORAGE_MESSAGES.configured }}</span>
            </span>
            <span
              v-else
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-medium leading-none bg-slate-100 text-slate-600 ring-1 ring-slate-200"
            >
              <span class="size-1.5 rounded-full bg-slate-400" />
              <span>{{ STORAGE_MESSAGES.unconfigured }}</span>
            </span>
          </template>
        </AdminColumn>

        <!-- 存储桶 / 路径 -->
        <AdminColumn
          field="bucket"
          :header="STORAGE_MESSAGES.bucketColumn"
          :style="{ width: '180px' }"
        >
          <template #body="{ data }">
            <span
              class="truncate font-mono text-xs"
              :class="data.bucket === '—' ? 'text-admin-text-3' : 'text-admin-text-1'"
            >
              {{ data.bucket }}
            </span>
          </template>
        </AdminColumn>

        <!-- 访问端点 / 域名 -->
        <AdminColumn
          field="endpoint"
          :header="STORAGE_MESSAGES.endpointColumn"
          :style="{ width: '220px' }"
        >
          <template #body="{ data }">
            <span
              class="truncate font-mono text-xs"
              :class="data.isPresetOnly ? 'text-admin-text-3' : 'text-admin-text-2'"
              :title="data.endpoint"
            >
              {{ data.endpoint }}
            </span>
          </template>
        </AdminColumn>

        <!-- 当前生效状态控制 -->
        <AdminColumn
          field="current"
          :header="STORAGE_MESSAGES.currentColumn"
          :style="{ width: '110px' }"
        >
          <template #body="{ data }">
            <span
              v-if="data.isCurrent"
              class="inline-flex items-center gap-1 font-mono text-xs font-semibold text-admin-success"
            >
              <Check class="size-3.5" />
              {{ STORAGE_MESSAGES.currentEffective }}
            </span>
            <AdminButton
              v-else-if="!data.isPresetOnly && data.storage"
              size="small"
              variant="outline"
              :class="adminActionButtonClass"
              :loading="applyingId === data.storage.id"
              :disabled="!data.canApply || applyingId !== null"
              @click="emit('apply-storage', data.storage)"
            >
              {{ STORAGE_MESSAGES.applyCurrent }}
            </AdminButton>
            <span v-else class="text-xs font-mono text-admin-text-3">—</span>
          </template>
        </AdminColumn>

        <!-- 操作列 (未配置显示“配置”，已配置显示“编辑/测试/删除”) -->
        <AdminColumn
          field="actions"
          :header="STORAGE_MESSAGES.actionsColumn"
          :style="{ width: '160px' }"
        >
          <template #body="{ data }">
            <div v-if="data.isPresetOnly" class="flex items-center gap-1.5">
              <AdminButton
                size="small"
                :class="adminActionButtonClass"
                @click="emit('configure-preset', data.presetKey)"
              >
                {{ STORAGE_MESSAGES.configureAction }}
              </AdminButton>
            </div>
            <div v-else-if="data.storage" class="flex items-center gap-1.5">
              <AdminButton
                size="small"
                variant="outline"
                :class="adminActionButtonClass"
                @click="emit('open-edit', data.storage.id)"
              >
                {{ STORAGE_MESSAGES.editAction }}
              </AdminButton>
              <AdminButton
                v-if="data.supportsTest"
                size="small"
                variant="outline"
                :class="adminActionButtonClass"
                :loading="testingId === data.storage.id"
                :disabled="!data.canTest || testingId !== null"
                @click="emit('test-storage', data.storage)"
              >
                {{ STORAGE_MESSAGES.testAction }}
              </AdminButton>
              <AdminButton
                v-if="!data.storage.isCurrent && !data.storage.builtin"
                size="small"
                variant="destructive"
                :class="adminActionButtonClass"
                @click="emit('remove-storage', data.storage)"
              >
                {{ STORAGE_MESSAGES.deleteAction }}
              </AdminButton>
            </div>
          </template>
        </AdminColumn>
      </AdminTable>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { Check, DatabaseExport, Search, Xmark } from '@iconoir/vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { STORAGE_MESSAGES } from '../pages/messages'
import type { StorageCard } from '../profiles'

export interface StorageTableRow {
  id: string
  isPresetOnly: boolean
  presetKey?: string | undefined
  storage?: StorageCard | undefined
  name: string
  subtitle: string
  providerLabel: string
  vendorLogo?: string | undefined
  vendorIcon?: Component | undefined
  avatarLabel?: string | undefined
  isCurrent: boolean
  statusText: '使用中' | '已配置' | '未配置'
  statusType: 'current' | 'configured' | 'unconfigured'
  bucket: string
  endpoint: string
  supportsTest: boolean
  canTest: boolean
  canApply: boolean
}

defineProps<{
  rows: StorageTableRow[]
  loading: boolean
  hasActiveFilter: boolean
  applyingId: string | null
  testingId: string | null
  activeLogoTargetId: string | null | undefined
}>()

const emit = defineEmits<{
  'clear-filters': []
  'open-logo-picker': [
    event: Event,
    card: { storage?: { id: string; name: string; config?: Record<string, unknown> } },
  ]
  'apply-storage': [storage: StorageCard]
  'configure-preset': [presetKey?: string]
  'open-edit': [id: string]
  'test-storage': [storage: StorageCard]
  'remove-storage': [storage: StorageCard]
}>()
</script>
