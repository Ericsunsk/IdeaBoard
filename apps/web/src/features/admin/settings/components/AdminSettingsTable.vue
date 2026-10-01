<template>
  <div :class="adminDashboardRootClass">
    <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
      <div class="flex items-center gap-2">
        <h2 class="text-admin-title font-bold text-admin-title-color">{{ title }}</h2>
        <span class="text-admin-meta text-admin-text-2">{{ subtitle }}</span>
      </div>
      <AdminButton
        variant="outline"
        size="small"
        :class="adminActionButtonClass"
        @click="toggleAllRows"
      >
        {{ SETTINGS_MESSAGES.tableExpandAll(isAllExpanded) }}
      </AdminButton>
    </div>

    <div :class="adminTableCardClass">
      <AdminTable
        v-model:expanded-rows="expandedRows"
        :value="rows"
        data-key="id"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
        :table-aria-label="tableAriaLabel"
        :scroll-aria-label="scrollAriaLabel"
      >
        <template #empty>
          <AdminEmptyState :title="SETTINGS_MESSAGES.emptyConfigs" />
        </template>

        <AdminColumn expander :style="{ width: '48px' }" />

        <AdminColumn
          field="title"
          :header="SETTINGS_MESSAGES.tableModuleColumn"
          :style="{ width: '280px', minWidth: '220px' }"
        >
          <template #body="{ data }">
            <AdminItemCell :icon="data.icon" :title="data.title" :description="data.description" />
          </template>
        </AdminColumn>

        <AdminColumn
          field="status"
          :header="SETTINGS_MESSAGES.statusColumn"
          :style="{ width: '140px' }"
        >
          <template #body="{ data }">
            <AdminBadge :value="data.status" :severity="data.statusSeverity" rounded />
          </template>
        </AdminColumn>

        <AdminColumn
          field="summary"
          :header="SETTINGS_MESSAGES.tableSummaryColumn"
          :style="{ minWidth: '280px' }"
        >
          <template #body="{ data }">
            <slot name="summary" :data="data">
              <span class="text-admin-body text-admin-text-2">{{ data.summary }}</span>
            </slot>
          </template>
        </AdminColumn>

        <AdminColumn
          v-if="hasQuickSwitchSlot"
          :header="SETTINGS_MESSAGES.tableQuickSwitchColumn"
          :style="{ width: '100px' }"
        >
          <template #body="{ data }">
            <slot name="quick-switch" :data="data" />
          </template>
        </AdminColumn>

        <AdminColumn :header="SETTINGS_MESSAGES.actionsColumn" :style="{ width: '90px' }">
          <template #body="{ data }">
            <AdminButton
              size="small"
              :variant="expandedRows[data.id] ? 'secondary' : 'outline'"
              :class="adminActionButtonClass"
              :aria-expanded="expandedRows[data.id] ? 'true' : 'false'"
              @click="toggleRow(data.id)"
            >
              {{ SETTINGS_MESSAGES.tableRowAction(Boolean(expandedRows[data.id])) }}
            </AdminButton>
          </template>
        </AdminColumn>

        <template #expansion="{ data, index }">
          <slot name="expansion" :data="data" :index="index" />
        </template>

        <template #after>
          <slot name="after" />
        </template>
      </AdminTable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useSlots, watch, type Component } from 'vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminItemCell from '../../shared/components/AdminItemCell.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import {
  adminActionButtonClass,
  adminDashboardRootClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { SETTINGS_MESSAGES } from '../pages/messages'

export interface AdminSettingsTableRow {
  id: string
  title: string
  description: string
  icon: Component
  status: string
  statusSeverity: 'danger' | 'info' | 'secondary' | 'success' | 'warn'
  summary: string
}

const props = withDefaults(
  defineProps<{
    title: string
    subtitle: string
    tableAriaLabel: string
    scrollAriaLabel: string
    rows?: readonly AdminSettingsTableRow[]
  }>(),
  { rows: () => [] },
)

const slots = useSlots()
const hasQuickSwitchSlot = computed(() => Boolean(slots['quick-switch']))
const rows = computed(() => props.rows)
const expandedRows = ref<Record<string, boolean>>(
  Object.fromEntries(props.rows.map((row) => [row.id, false])),
)

watch(
  () => props.rows.map((row) => row.id),
  (ids) => {
    const next: Record<string, boolean> = {}
    for (const id of ids) {
      next[id] = expandedRows.value[id] ?? false
    }
    expandedRows.value = next
  },
)

const isAllExpanded = computed(() => {
  return rows.value.length > 0 && rows.value.every((row) => expandedRows.value[row.id])
})

function toggleAllRows() {
  const target = !isAllExpanded.value
  const next: Record<string, boolean> = {}
  for (const row of rows.value) {
    next[row.id] = target
  }
  expandedRows.value = next
}

function toggleRow(id: string) {
  expandedRows.value = {
    ...expandedRows.value,
    [id]: !expandedRows.value[id],
  }
}
</script>
