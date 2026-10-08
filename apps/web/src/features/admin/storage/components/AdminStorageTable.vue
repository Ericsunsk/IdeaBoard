<template>
  <!-- 加载中状态 (AdminSpinner 自带 role="progressbar") -->
  <div
    v-if="loading && rows.length === 0"
    class="flex min-h-[260px] items-center justify-center rounded-xl border border-border bg-card text-muted-foreground"
    :aria-label="STORAGE_MESSAGES.tableLoadingAria"
  >
    <AdminSpinner class="size-9 text-primary" />
  </div>

  <!-- 空状态提示 (无数据或过滤未命中) -->
  <div
    v-else-if="rows.length === 0"
    class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-border bg-card p-10 text-center"
  >
    <div
      class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground"
    >
      <DatabaseExport class="size-6" />
    </div>
    <h3 class="text-base font-semibold text-foreground">
      {{ STORAGE_MESSAGES.tableEmptyTitle }}
    </h3>
    <p class="mt-1 max-w-sm text-xs text-muted-foreground">
      {{ STORAGE_MESSAGES.tableEmptyHint }}
    </p>
  </div>

  <!-- 存储配置表格 (1:1 对齐功能定价页 BillingPricingTable 规范，预设厂商平铺展示) -->
  <section v-else :aria-label="STORAGE_MESSAGES.tableSectionAria">
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

        <!-- 存储服务列 (图标/Logo + 名称) -->
        <AdminColumn
          field="service"
          :header="STORAGE_MESSAGES.serviceColumn"
          :style="{ width: '200px', minWidth: '170px' }"
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
                  class="size-4 text-muted-foreground"
                />
                <span
                  v-else
                  class="text-admin-caption font-bold text-primary uppercase select-none"
                >
                  {{ data.avatarLabel }}
                </span>
              </button>
              <div
                v-else
                class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card p-1.5 shadow-xs"
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
                  class="size-4 text-muted-foreground"
                />
                <span
                  v-else
                  class="text-admin-caption font-bold text-muted-foreground uppercase select-none"
                >
                  {{ data.avatarLabel }}
                </span>
              </div>
              <span
                class="truncate text-xs font-bold"
                :class="data.isPresetOnly ? 'text-muted-foreground' : 'text-foreground'"
              >
                {{ data.name }}
              </span>
            </div>
          </template>
        </AdminColumn>

        <!-- Status 列（全局规范：任务监控页 TaskStatusCell 式 Token 胶囊 + 圆点） -->
        <AdminColumn field="status" header="Status" :style="{ width: '105px' }">
          <template #body="{ data }">
            <span
              v-if="data.isCurrent"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-semibold leading-none bg-success-soft text-success ring-1 ring-success/20"
            >
              <span class="size-1.5 rounded-full bg-success" />
              <span>{{ STORAGE_MESSAGES.current }}</span>
            </span>
            <span
              v-else-if="data.statusType === 'configured'"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-semibold leading-none bg-info-soft text-info ring-1 ring-info/20"
            >
              <span class="size-1.5 rounded-full bg-info" />
              <span>{{ STORAGE_MESSAGES.configured }}</span>
            </span>
            <span
              v-else
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-semibold leading-none bg-muted text-muted-foreground ring-1 ring-border"
            >
              <span class="size-1.5 rounded-full bg-muted-foreground" />
              <span>{{ STORAGE_MESSAGES.unconfigured }}</span>
            </span>
          </template>
        </AdminColumn>

        <!-- 存储桶 / 路径 -->
        <AdminColumn
          field="bucket"
          :header="STORAGE_MESSAGES.bucketColumn"
          :style="{ width: '180px', minWidth: '140px' }"
        >
          <template #body="{ data }">
            <span
              class="truncate font-mono text-xs"
              :class="data.bucket === '—' ? 'text-muted-foreground' : 'text-foreground'"
            >
              {{ data.bucket }}
            </span>
          </template>
        </AdminColumn>

        <!-- 访问端点 / 域名 (核心自适应伸缩列) -->
        <AdminColumn
          field="endpoint"
          :header="STORAGE_MESSAGES.endpointColumn"
          :style="{ minWidth: '220px' }"
        >
          <template #body="{ data }">
            <span class="truncate font-mono text-xs text-muted-foreground" :title="data.endpoint">
              {{ data.endpoint }}
            </span>
          </template>
        </AdminColumn>

        <!-- 操作列 (与表头严格左对齐) -->
        <AdminColumn
          field="actions"
          :header="STORAGE_MESSAGES.actionsColumn"
          :style="{ width: '220px', minWidth: '180px' }"
        >
          <template #body="{ data }">
            <div v-if="data.isPresetOnly" class="flex items-center gap-1.5">
              <ActionButton @click="emit('configure-preset', data.presetKey)">
                {{ STORAGE_MESSAGES.configureAction }}
              </ActionButton>
            </div>
            <div v-else-if="data.storage" class="flex items-center gap-1.5">
              <ActionButton
                v-if="!data.storage.isCurrent"
                variant="outline"
                :loading="applyingId === data.storage.id"
                :disabled="!data.canApply || applyingId !== null"
                @click="emit('apply-storage', data.storage)"
              >
                {{ STORAGE_MESSAGES.applyCurrent }}
              </ActionButton>
              <ActionButton variant="outline" @click="emit('open-edit', data.storage.id)">
                {{ STORAGE_MESSAGES.configureAction }}
              </ActionButton>
              <ActionButton
                v-if="data.supportsTest"
                variant="outline"
                :loading="testingId === data.storage.id"
                :disabled="!data.canTest || testingId !== null"
                @click="emit('test-storage', data.storage)"
              >
                {{ STORAGE_MESSAGES.testAction }}
              </ActionButton>
              <ActionButton
                v-if="!data.storage.isCurrent && !data.storage.builtin"
                variant="destructive"
                @click="emit('remove-storage', data.storage)"
              >
                {{ STORAGE_MESSAGES.deleteAction }}
              </ActionButton>
            </div>
          </template>
        </AdminColumn>

        <!-- 启用状态列 -->
        <AdminColumn
          field="enabled"
          :header="STORAGE_MESSAGES.enableColumn"
          :style="{ width: '80px' }"
        >
          <template #body="{ data }">
            <div
              class="flex items-center"
              :title="data.storage && storageDisableReason(data.storage)"
            >
              <AdminSwitch
                :model-value="data.enabled"
                :aria-label="`启用 ${data.name}`"
                :aria-description="data.storage && storageDisableReason(data.storage)"
                :disabled="
                  data.isPresetOnly ||
                  !data.storage ||
                  Boolean(storageDisableReason(data.storage)) ||
                  togglingId != null
                "
                @update:model-value="
                  data.storage && emit('toggle-enabled', data.storage, Boolean($event))
                "
              />
            </div>
          </template>
        </AdminColumn>
      </AdminTable>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { DatabaseExport } from '@iconoir/vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import {
  adminLogoButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { STORAGE_MESSAGES } from '../pages/messages'
import { storageDisableReason, type StorageCard } from '../profiles'

export interface StorageTableRow {
  id: string
  isPresetOnly: boolean
  presetKey?: string | undefined
  storage?: StorageCard | undefined
  name: string
  vendorLogo?: string | undefined
  vendorIcon?: Component | undefined
  avatarLabel?: string | undefined
  isCurrent: boolean
  enabled: boolean
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
  applyingId: string | null
  testingId: string | null
  togglingId?: string | null | undefined
  activeLogoTargetId: string | null | undefined
}>()

const emit = defineEmits<{
  'open-logo-picker': [
    event: Event,
    card: { storage?: { id: string; name: string; config?: Record<string, unknown> } },
  ]
  'apply-storage': [storage: StorageCard]
  'configure-preset': [presetKey?: string]
  'open-edit': [id: string]
  'test-storage': [storage: StorageCard]
  'remove-storage': [storage: StorageCard]
  'toggle-enabled': [storage: StorageCard, enabled: boolean]
}>()
</script>
