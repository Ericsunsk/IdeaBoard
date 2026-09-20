<template>
  <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
    <!-- 左侧：侧边栏抽屉按钮 + Events 选择器 + 搜索输入框 -->
    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
      <!-- 侧边栏按钮 (样式占位，后续接功能) -->
      <button
        type="button"
        aria-disabled="true"
        tabindex="-1"
        class="relative flex size-9 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-white text-admin-text-2"
        title="切换侧边导航"
      >
        <SidebarCollapse class="size-4" />
        <span class="absolute top-1.5 right-1.5 size-1.5 rounded-full bg-blue-500"></span>
      </button>

      <!-- 数据集选择器 (样式占位，后续接功能) -->
      <div class="relative inline-flex items-center">
        <button
          type="button"
          aria-disabled="true"
          tabindex="-1"
          class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-semibold text-admin-text-1"
        >
          <Table class="size-3.5 text-admin-text-2" />
          <span>Events</span>
          <NavArrowDown class="size-3.5 text-admin-text-3" />
        </button>
      </div>

      <!-- 搜索输入框: 🔍 使用查询语言搜索... ❓ -->
      <div class="relative flex min-w-[200px] flex-1 items-center">
        <div class="pointer-events-none absolute left-3 flex items-center text-admin-text-3">
          <Search class="size-4" />
        </div>
        <input
          :value="searchKeyword"
          type="text"
          :aria-label="t('admin.common.search')"
          class="h-9 w-full rounded-lg border border-admin-border-soft bg-white pr-9 pl-9 text-xs text-admin-text-1 placeholder:text-admin-text-3 transition-colors focus-visible:border-admin-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20"
          :placeholder="t('admin.common.search')"
          @input="emit('update:searchKeyword', ($event.target as HTMLInputElement).value)"
          @keydown.enter="emit('search')"
        />
        <div class="pointer-events-none absolute right-3 flex items-center text-admin-text-3">
          <HelpCircle class="size-4" />
        </div>
      </div>
    </div>

    <!-- 右侧：▶ 实时 + 📅 过去 1 小时 + ··· 更多 + 分页器 -->
    <div class="flex shrink-0 items-center gap-2">
      <!-- 实时按钮 -->
      <button
        type="button"
        class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        @click="emit('liveRefresh')"
      >
        <Play class="size-3 fill-admin-text-1 text-admin-text-1" />
        <span>{{ t('admin.monitor.autoRefresh') }}</span>
      </button>

      <!-- 时间范围下拉按钮与弹出层 (shadcn-vue 原生原子组件) -->
      <AdminPopover
        :open="timeRangePopoverOpen"
        @update:open="emit('update:timeRangePopoverOpen', $event)"
      >
        <AdminPopoverTrigger as-child>
          <button
            type="button"
            class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
            :aria-expanded="timeRangePopoverOpen"
            aria-haspopup="listbox"
            aria-label="选择时间范围"
          >
            <Calendar class="size-3.5 text-admin-text-2" />
            <span>{{ selectedPreset.label }}</span>
            <NavArrowDown
              class="size-3.5 text-admin-text-3 transition-transform"
              :class="timeRangePopoverOpen ? 'rotate-180' : ''"
            />
          </button>
        </AdminPopoverTrigger>

        <AdminPopoverContent align="end" :side-offset="4">
          <div class="flex flex-col gap-0.5" role="listbox">
            <button
              v-for="preset in timeRangePresets"
              :key="preset.key"
              type="button"
              role="option"
              :aria-selected="selectedTimeRangeKey === preset.key"
              :class="[
                'flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer',
                selectedTimeRangeKey === preset.key
                  ? 'bg-blue-50 text-blue-600 font-semibold'
                  : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
              ]"
              @click="emit('selectTimeRange', preset.key)"
            >
              <span>{{ preset.label }}</span>
              <Check v-if="selectedTimeRangeKey === preset.key" class="size-3.5 text-blue-600 shrink-0" />
            </button>
          </div>
        </AdminPopoverContent>
      </AdminPopover>

      <!-- 更多按钮 (样式占位，后续接功能) -->
      <AdminButton
        variant="ghost"
        class="size-9 rounded-lg border border-admin-border-soft bg-white p-0 text-admin-text-2"
        aria-label="更多操作"
        aria-disabled="true"
        tabindex="-1"
      >
        <MoreHoriz class="size-4 text-admin-text-2" />
      </AdminButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  AdminPopover,
  AdminPopoverContent,
  AdminPopoverTrigger,
} from '../../../shared/components/adminPopoverAtoms'
import AdminButton from '../../../shared/components/AdminButton.vue'
import {
  Calendar,
  Check,
  HelpCircle,
  MoreHoriz,
  NavArrowDown,
  Play,
  Search,
  SidebarCollapse,
  Table,
} from '@iconoir/vue'
import type { TimeRangePreset, TimeRangePresetKey } from '../../domain/timeRange'

defineProps<{
  searchKeyword: string
  timeRangePopoverOpen: boolean
  selectedPreset: TimeRangePreset
  selectedTimeRangeKey: TimeRangePresetKey
  timeRangePresets: readonly TimeRangePreset[]
}>()

const emit = defineEmits<{
  'update:searchKeyword': [value: string]
  'update:timeRangePopoverOpen': [value: boolean]
  search: []
  liveRefresh: []
  selectTimeRange: [key: TimeRangePresetKey]
}>()

const { t } = useI18n()
</script>
