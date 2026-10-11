<template>
  <!-- 存储配置表格 (1:1 对齐功能定价页 BillingPricingTable 规范，预设厂商平铺展示) -->
  <!-- rows 恒含厂商预设行：初载走 AdminTable 内部 loading 行，过滤未命中走 #empty，此处不再套双层状态。 -->
  <section :aria-label="STORAGE_MESSAGES.tableSectionAria">
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

        <!-- Service 列 (图标/Logo + 名称) -->
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

        <!-- Status 列（健康三态：正常/异常/未验证，载体统一 AdminStatusPill） -->
        <AdminColumn field="status" :header="STATUS_COLUMN_HEADER" :style="{ width: '105px' }">
          <template #body="{ data }">
            <AdminStatusPill
              :label="storageHealthLabel(data.health)"
              :variant="storageHealthVariant(data.health)"
              size="small"
            />
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

        <!-- 启用状态列（范式：启用开关在操作列之前，操作列永远最右） -->
        <!-- 操作列 (与表头严格左对齐，永远最右) -->
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
              <!-- toggling 只锁定本行开关：其他行保持可交互，避免整列同时变灰闪烁。 -->
              <AdminSwitch
                :model-value="data.enabled"
                :aria-label="`Enable ${data.name}`"
                :aria-description="data.storage && storageDisableReason(data.storage)"
                :disabled="
                  data.isPresetOnly ||
                  !data.storage ||
                  Boolean(storageDisableReason(data.storage)) ||
                  (data.storage ? isRowToggling(data.storage.id) : false)
                "
                @update:model-value="
                  data.storage && emit('toggle-enabled', data.storage, Boolean($event))
                "
              />
            </div>
          </template>
        </AdminColumn>
      </AdminTable>
      <div v-if="$slots.footer" :class="adminTableFooterClass">
        <slot name="footer" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import ActionButton from '../../../../../../core/components/ActionButton.vue'
import { AdminColumn } from '../../../../shared/index'
import { AdminEmptyState } from '../../../../shared/index'
import { AdminStatusPill } from '../../../../shared/index'
import { AdminSwitch } from '../../../../shared/index'
import { AdminTable } from '../../../../shared/index'
import {
  adminLogoButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFooterClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../../../shared/index'
import { STORAGE_MESSAGES } from './messages'
import { storageDisableReason, type StorageCard } from '../../../core/storage/profiles'

const props = defineProps<{
  rows: StorageTableRow[]
  loading: boolean
  applyingId: string | null
  testingId: string | null
  isToggling?: ((id: string) => boolean) | undefined
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

function isRowToggling(id: string): boolean {
  return props.isToggling?.(id) ?? false
}

// 健康三态（方案 A）：Ready/异常/未验证；启用只用开关，无启用胶囊；
// 配置缺失只占位“—”。组件内常量，不进 messages.ts 公共文案。
export type StorageHealth = 'normal' | 'abnormal' | 'unverified'
const STATUS_COLUMN_HEADER = 'Status'
const STORAGE_HEALTH_LABEL: Record<StorageHealth, string> = {
  normal: 'Ready',
  abnormal: 'Error',
  unverified: 'Unverified',
}
const STORAGE_HEALTH_VARIANT: Record<StorageHealth, 'success' | 'danger' | 'neutral'> = {
  normal: 'success',
  abnormal: 'danger',
  unverified: 'neutral',
}

function storageHealthLabel(health: StorageHealth): string {
  return STORAGE_HEALTH_LABEL[health]
}

function storageHealthVariant(health: StorageHealth): 'success' | 'danger' | 'neutral' {
  return STORAGE_HEALTH_VARIANT[health]
}

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
  health: StorageHealth
  bucket: string
  endpoint: string
  supportsTest: boolean
  canTest: boolean
  canApply: boolean
}
</script>
