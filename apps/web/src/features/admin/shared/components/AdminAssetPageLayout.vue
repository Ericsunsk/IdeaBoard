<template>
  <div class="admin-asset-page-layout">
    <div class="page-header">
      <div>
        <h2 class="page-title">{{ title }}</h2>
        <p class="page-desc">{{ description }}</p>
      </div>
      <div class="header-side-note">
        <span>{{ totalLabel }} {{ total }}</span>
      </div>
    </div>

    <div class="summary-grid">
      <slot name="summary" />
    </div>

    <div class="toolbar">
      <slot name="toolbar" />
    </div>

    <div class="table-card">
      <AdminSectionHeader :title="tableTitle" :description="tableDescription" />
      <slot name="table" />
      <div class="pagination-wrapper">
        <slot name="pagination" />
      </div>
    </div>

    <slot name="modals" />
  </div>
</template>

<script setup lang="ts">
import AdminSectionHeader from './AdminSectionHeader.vue'

withDefaults(defineProps<{
  title: string
  description: string
  total: number | string
  totalLabel?: string
  tableTitle: string
  tableDescription?: string
}>(), {
  totalLabel: '总数',
  tableDescription: '',
})
</script>

<style scoped>
@import '../adminCommon.css';

.admin-asset-page-layout {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
</style>
