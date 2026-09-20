<template>
  <div :class="adminDashboardRootClass">
    <!-- 顶栏：左侧搜索与主操作，右侧筛选与刷新（与用户管理页同一样式） -->
    <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
      <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
        <slot name="toolbar-start" />
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <slot name="toolbar-end" />
      </div>
    </div>

    <!-- 表格卡片：灰底容器 + 告警 + 表格 + 灰底底栏分页 -->
    <div :class="adminTableCardClass">
      <slot name="alerts" />
      <slot />
      <div :class="adminTableFooterClass">
        <span class="text-xs font-normal text-admin-text-2-strong">
          <slot name="footer-label" />
        </span>
        <div class="flex items-center gap-2">
          <AdminPagerControls
            :current-page="currentPage"
            :total-pages="totalPages"
            :prev-disabled="prevDisabled"
            :next-disabled="nextDisabled"
            @prev="emit('prev')"
            @next="emit('next')"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import AdminPagerControls from './AdminPagerControls.vue'
import {
  adminDashboardRootClass,
  adminTableCardClass,
  adminTableFooterClass,
} from '../lib/uiClasses'

defineProps<{
  currentPage: number
  totalPages: number
  prevDisabled: boolean
  nextDisabled: boolean
}>()

const emit = defineEmits<{
  prev: []
  next: []
}>()
</script>
