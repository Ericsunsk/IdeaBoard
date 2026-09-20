<template>
  <AdminSettingsTable
    title="邮件服务"
    subtitle="SMTP 事务邮件发送服务与连通性测试"
    table-aria-label="邮件服务设置列表"
    scroll-aria-label="邮件服务设置列表横向滚动区域"
    :rows="settingRows"
  >
    <template #expansion="{ data }">
      <div class="border-t border-admin-border-soft bg-admin-bg p-4 md:p-5">
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

const settingRows = computed<AdminSettingsTableRow[]>(() => [
  {
    id: 'mail',
    title: '邮件发送',
    description: 'SMTP 事务邮件发送服务与测试',
    icon: Mail,
    status: '邮件配置',
    statusSeverity: 'secondary',
    summary: '配置发件人、发件邮箱、密码、端口与邮件连通性测试',
  },
])
</script>
