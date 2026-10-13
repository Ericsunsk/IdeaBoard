<template>
  <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
    <!-- 左侧：侧边栏抽屉按钮 + Events 选择器 + 搜索输入框 -->
    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
      <!-- 侧边栏按钮 (样式占位，后续接功能) -->
      <button
        type="button"
        disabled
        class="relative flex size-9 shrink-0 cursor-not-allowed items-center justify-center rounded-control border border-border bg-card text-muted-foreground opacity-60"
        :title="MONITOR_MESSAGES.sidebarToggleTitle"
      >
        <SidebarCollapse class="size-4" />
        <span class="absolute top-1.5 right-1.5 size-1.5 rounded-pill bg-info"></span>
      </button>

      <!-- 数据集选择器 (样式占位，后续接功能) -->
      <div class="relative inline-flex items-center">
        <button
          type="button"
          disabled
          class="flex h-9 cursor-not-allowed items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-semibold text-foreground opacity-60"
        >
          <Table class="size-3.5 text-muted-foreground" />
          <span>Events</span>
          <NavArrowDown class="size-3.5 text-muted-foreground" />
        </button>
      </div>

      <!-- 搜索输入框: AdminToolbarSearch（带清空按钮，清空即重新查询） -->
      <AdminToolbarSearch
        :model-value="searchKeyword"
        :input-label="t('admin.common.search')"
        :placeholder="t('admin.common.search')"
        fluid
        @update:model-value="emit('update:searchKeyword', $event)"
        @clear="emit('search')"
        @enter="emit('search')"
      />
    </div>

    <!-- 右侧：▶ 实时 + 📅 过去 1 小时 + ··· 更多 + 分页器 -->
    <div class="flex shrink-0 items-center gap-2">
      <!-- 实时按钮 -->
      <button
        type="button"
        class="flex h-9 items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors hover:bg-muted cursor-pointer"
        @click="emit('liveRefresh')"
      >
        <Play class="size-3 fill-foreground text-foreground" />
        <span>{{ t('admin.monitor.autoRefresh') }}</span>
      </button>

      <!--
        Time-range picker. Builds on AdminSelect (reka `Select`): the previous
        implementation was an `AdminPopover` with `aria-haspopup="listbox"` over a
        hand-rolled `role="listbox"`, which advertised arrow keys, Home/End and
        typeahead without implementing one of them. The keyboard model and the
        Admin skin now come from the shared primitive, so the trigger is a single
        Tab stop. `placeholder` keeps the resolved `selectedPreset.label` visible
        when the current key is not among the presets, as the old trigger did.
      -->
      <AdminSelect
        :model-value="selectedTimeRangeKey"
        :options="timeRangeOptions"
        :placeholder="selectedPreset.label"
        :aria-label="MONITOR_MESSAGES.timeRangeAria"
        class="w-auto"
        @update:model-value="handleTimeRangeUpdate"
      />

      <!-- 更多按钮 (样式占位，后续接功能) -->
      <AdminButton
        variant="ghost"
        class="size-9 rounded-control border border-border bg-card p-0 text-muted-foreground"
        :aria-label="MONITOR_MESSAGES.moreActionsAria"
        disabled
      >
        <MoreHoriz class="size-4 text-muted-foreground" />
      </AdminButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { AdminButton } from '../../../shared'
import { AdminSelect } from '../../../shared'
import { AdminToolbarSearch } from '../../../shared'
import { MoreHoriz, NavArrowDown, Play, SidebarCollapse, Table } from '@iconoir/vue'
import type { TimeRangePreset, TimeRangePresetKey } from '../../core/timeRange'
import { MONITOR_MESSAGES } from '../pages/messages'

const props = defineProps<{
  searchKeyword: string
  selectedPreset: TimeRangePreset
  selectedTimeRangeKey: TimeRangePresetKey
  timeRangePresets: readonly TimeRangePreset[]
}>()

const emit = defineEmits<{
  'update:searchKeyword': [value: string]
  search: []
  liveRefresh: []
  selectTimeRange: [key: TimeRangePresetKey]
}>()

const timeRangeOptions = computed(() =>
  props.timeRangePresets.map((preset) => ({ label: preset.label, value: preset.key })),
)

function handleTimeRangeUpdate(value: string | number) {
  emit('selectTimeRange', String(value) as TimeRangePresetKey)
}

const { t } = useI18n()
</script>
