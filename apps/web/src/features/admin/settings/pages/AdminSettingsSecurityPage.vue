<template>
  <AdminSettingsTable
    title="安全与网络"
    subtitle="用户准入限制、API 访问速率与私网出网策略"
    table-aria-label="安全与网络设置列表"
    scroll-aria-label="安全与网络设置列表横向滚动区域"
    :rows="settingRows"
  >
    <template #quick-switch="{ data }">
      <div v-if="data.id === 'registration_ratelimit'" class="flex items-center">
        <AdminSwitch
          id="admin-table-registration-enabled"
          v-model="registrationEnabled"
          :disabled="registrationSettingLoading"
          aria-label="快速切换开放注册"
          @update:model-value="handleToggleRegistration"
        />
      </div>
      <span v-else class="text-xs text-admin-text-3/50 select-none">—</span>
    </template>

    <template #expansion="{ data }">
      <div class="border-t border-admin-border-soft bg-admin-bg p-4 md:p-5">
        <!-- 1. 注册与 API 限流编辑表单 -->
        <SettingEditorCard
          v-if="data.id === 'registration_ratelimit'"
          title="注册准入与 API 限流"
          :status="registrationEnabled ? '开放注册' : '关闭注册'"
          :status-severity="registrationEnabled ? 'success' : 'warn'"
          class="mb-0"
        >
          <div :class="settingCardBodyClass">
            <div
              class="flex min-h-12 items-center justify-between gap-4 rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
            >
              <div class="flex flex-col">
                <span class="text-admin-body font-bold text-admin-text-1">开放注册</span>
                <span class="text-admin-caption text-admin-text-3"
                  >允许新用户自行注册账号；关闭后仅系统管理员可创建用户</span
                >
              </div>
              <AdminSwitch
                id="admin-registration-enabled"
                v-model="registrationEnabled"
                :disabled="registrationSettingLoading"
                aria-label="开放注册"
                @update:model-value="handleToggleRegistration"
              />
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <AdminFormField label="最大请求 (次)" for="admin-rate-limit-max-requests">
                <AdminInput
                  id="admin-rate-limit-max-requests"
                  v-model="rateLimitForm.max_requests"
                  type="number"
                  :min="1"
                  :max="RATE_LIMIT_MAX_REQUESTS"
                  :disabled="rateLimitLoading || rateLimitSaving"
                />
              </AdminFormField>

              <AdminFormField label="窗口周期 (秒)" for="admin-rate-limit-window-seconds">
                <AdminInput
                  id="admin-rate-limit-window-seconds"
                  v-model="rateLimitForm.window_seconds"
                  type="number"
                  :min="1"
                  :max="RATE_LIMIT_MAX_WINDOW_SECONDS"
                  :disabled="rateLimitLoading || rateLimitSaving"
                />
              </AdminFormField>
            </div>
          </div>
          <template #actions>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              :disabled="rateLimitLoading || rateLimitSaving"
              @click="resetRateLimit"
            >
              重置
            </AdminButton>
            <AdminButton
              :class="adminActionButtonClass"
              :disabled="rateLimitLoading || rateLimitSaving"
              :loading="rateLimitSaving"
              @click="saveRateLimit"
            >
              保存
            </AdminButton>
          </template>
        </SettingEditorCard>

        <!-- 2. 私有地址访问 -->
        <AdminOutboundPolicyCard v-else-if="data.id === 'outbound_policy'" class="mb-0" />
      </div>
    </template>
  </AdminSettingsTable>
</template>

<script setup lang="ts">
import { Server, ShieldCheck } from '@iconoir/vue'
import { computed } from 'vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminOutboundPolicyCard from '../AdminOutboundPolicyCard.vue'
import SettingEditorCard from '../SettingEditorCard.vue'
import AdminSettingsTable, {
  type AdminSettingsTableRow,
} from '../components/AdminSettingsTable.vue'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  useAccessSettings,
} from '../useAccessSettings'
import { useAdminAccessSettingsFeedback } from '../useSettingsFeedback'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'

const settingCardBodyClass = 'flex flex-col gap-5'

const access = useAccessSettings({ feedback: useAdminAccessSettingsFeedback() })

const {
  rateLimitForm,
  rateLimitLoading,
  registrationEnabled,
  registrationSettingLoading,
  rateLimitSaving,
  resetRateLimit,
  saveRateLimit,
  handleToggleRegistration,
} = access

function buildRegistrationRow(
  enabled: boolean,
  form: typeof rateLimitForm.value,
): AdminSettingsTableRow {
  const maxRequests = form?.max_requests ?? 120
  const windowSeconds = form?.window_seconds ?? 60
  return {
    id: 'registration_ratelimit',
    title: '注册准入与 API 限流',
    description: '开放注册开关与全局请求速率保护',
    icon: ShieldCheck,
    status: enabled ? '开放注册' : '关闭注册',
    statusSeverity: enabled ? 'success' : 'warn',
    summary: `准入: ${enabled ? '开放注册' : '关闭注册'} · 限流: 最大 ${maxRequests} 次 / ${windowSeconds} 秒`,
  }
}

const staticOutboundRow: AdminSettingsTableRow = {
  id: 'outbound_policy',
  title: '私有地址访问',
  description: '安全出网私有网络与专有主机放行白名单',
  icon: Server,
  status: '私网放行策略',
  statusSeverity: 'secondary',
  summary: '配置允许服务端直接发起的内部 IP / 域名访问策略',
}

const settingRows = computed<AdminSettingsTableRow[]>(() => [
  buildRegistrationRow(registrationEnabled.value, rateLimitForm.value),
  staticOutboundRow,
])
</script>
