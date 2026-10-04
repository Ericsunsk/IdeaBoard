<template>
  <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
    <!-- 左侧：侧边栏抽屉按钮 + Events 选择器 + 搜索输入框 -->
    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
      <!-- 侧边栏按钮 (样式占位，后续接功能) -->
      <button
        type="button"
        disabled
        class="relative flex size-9 shrink-0 cursor-not-allowed items-center justify-center rounded-lg border border-border bg-card text-muted-foreground opacity-60"
        :title="MONITOR_MESSAGES.sidebarToggleTitle"
      >
        <SidebarCollapse class="size-4" />
        <span class="absolute top-1.5 right-1.5 size-1.5 rounded-full bg-sky-500"></span>
      </button>

      <!-- 数据集选择器 (样式占位，后续接功能) -->
      <div class="relative inline-flex items-center">
        <button
          type="button"
          disabled
          class="flex h-9 cursor-not-allowed items-center gap-1.5 rounded-lg border border-border bg-card px-3 text-xs font-semibold text-foreground opacity-60"
        >
          <Table class="size-3.5 text-muted-foreground" />
          <span>Events</span>
          <NavArrowDown class="size-3.5 text-muted-foreground" />
        </button>
      </div>

      <!-- 搜索输入框: 🔍 使用查询语言搜索... ❓ -->
      <div class="relative flex min-w-[200px] flex-1 items-center">
        <div class="pointer-events-none absolute left-3 flex items-center text-muted-foreground">
          <Search class="size-4" />
        </div>
        <input
          :value="searchKeyword"
          type="text"
          :aria-label="t('admin.common.search')"
          class="h-9 w-full rounded-lg border border-input bg-background pr-9 pl-9 text-xs text-foreground placeholder:text-muted-foreground transition-colors focus-visible:border-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20"
          :placeholder="t('admin.common.search')"
          @input="emit('update:searchKeyword', ($event.target as HTMLInputElement).value)"
          @keydown.enter="emit('search')"
        />
        <div class="pointer-events-none absolute right-3 flex items-center text-muted-foreground">
          <HelpCircle class="size-4" />
        </div>
      </div>
    </div>

    <!-- 右侧：▶ 实时 + 📅 过去 1 小时 + ··· 更多 + 分页器 -->
    <div class="flex shrink-0 items-center gap-2">
      <!-- 实时按钮 -->
      <button
        type="button"
        class="flex h-9 items-center gap-1.5 rounded-lg border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors hover:bg-muted cursor-pointer"
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
        class="size-9 rounded-lg border border-border bg-card p-0 text-muted-foreground"
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
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminSelect from '../../../shared/components/AdminSelect.vue'
import {
  HelpCircle,
  MoreHoriz,
  NavArrowDown,
  Play,
  Search,
  SidebarCollapse,
  Table,
} from '@iconoir/vue'
import type { TimeRangePreset, TimeRangePresetKey } from '../../domain/timeRange'
import { MONITOR_MESSAGES } from '../pages/messages'

const props = defineProps<{
  searchKeyword: string
  /**
   * @deprecated reka `Select` owns its own open state. `AdminMonitorPage` still
   * binds `v-model:time-range-popover-open` to this pair; dropping it means
   * touching `useMonitorTimeline` and the page as well, which is out of scope
   * here, so the contract is kept inert rather than deleted mid-refactor.
   */
  timeRangePopoverOpen: boolean
  selectedPreset: TimeRangePreset
  selectedTimeRangeKey: TimeRangePresetKey
  timeRangePresets: readonly TimeRangePreset[]
}>()

const emit = defineEmits<{
  'update:searchKeyword': [value: string]
  /** @deprecated See `timeRangePopoverOpen`; the dropdown no longer reports open state. */
  'update:timeRangePopoverOpen': [value: boolean]
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
