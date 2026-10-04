<template>
  <AdminSettingsTable
    :title="SETTINGS_MESSAGES.mailTitle"
    :subtitle="SETTINGS_MESSAGES.mailSubtitle"
    :table-aria-label="SETTINGS_MESSAGES.mailTableAria"
    :scroll-aria-label="SETTINGS_MESSAGES.mailTableScrollAria"
    :rows="settingRows"
  >
    <template #expansion="{ data }">
      <div class="border-t border-border bg-card p-4 md:p-5">
        <AdminMailConfigCard v-if="data.id === 'mail'" class="mb-0" />
      </div>
    </template>
  </AdminSettingsTable>
</template>

<script setup lang="ts">
import { Mail } from '@iconoir/vue'
import { computed } from 'vue'
import AdminSettingsTable, {
  type AdminSettingsTableRow,
} from '../components/AdminSettingsTable.vue'
import AdminMailConfigCard from '../AdminMailConfigCard.vue'
import { SETTINGS_MESSAGES } from './messages'

const settingRows = computed<AdminSettingsTableRow[]>(() => [
  {
    id: 'mail',
    title: SETTINGS_MESSAGES.mailRowTitle,
    description: SETTINGS_MESSAGES.mailRowDescription,
    icon: Mail,
    status: SETTINGS_MESSAGES.mailRowStatus,
    statusSeverity: 'secondary',
    summary: SETTINGS_MESSAGES.mailRowSummary,
  },
])
</script>
