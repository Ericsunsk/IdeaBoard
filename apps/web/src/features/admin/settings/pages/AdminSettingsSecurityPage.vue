<template>
  <AdminSettingsTable
    :title="SETTINGS_MESSAGES.securityTitle"
    :subtitle="SETTINGS_MESSAGES.securitySubtitle"
    :table-aria-label="SETTINGS_MESSAGES.securityTableAria"
    :scroll-aria-label="SETTINGS_MESSAGES.securityTableScrollAria"
    :rows="settingRows"
  >
    <template #quick-switch="{ data }">
      <div v-if="data.id === 'registration_ratelimit'" class="flex items-center">
        <AdminSwitch
          id="admin-table-registration-enabled"
          v-model="registrationEnabled"
          :disabled="registrationSettingLoading"
          :aria-label="SETTINGS_MESSAGES.quickToggleRegistration"
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
          :title="SETTINGS_MESSAGES.registrationTitle"
          :status="registrationEnabled ? SETTINGS_MESSAGES.registrationOpen : SETTINGS_MESSAGES.registrationClosed"
          :status-severity="registrationEnabled ? 'success' : 'warn'"
          class="mb-0"
        >
          <div :class="settingCardBodyClass">
            <div
              class="flex min-h-12 items-center justify-between gap-4 rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
            >
              <div class="flex flex-col">
                <span class="text-admin-body font-bold text-admin-text-1">{{ SETTINGS_MESSAGES.registrationOpen }}</span>
                <span class="text-admin-caption text-admin-text-3"
                  >{{ SETTINGS_MESSAGES.registrationHint }}</span
                >
              </div>
              <AdminSwitch
                id="admin-registration-enabled"
                v-model="registrationEnabled"
                :disabled="registrationSettingLoading"
                :aria-label="SETTINGS_MESSAGES.registrationOpen"
                @update:model-value="handleToggleRegistration"
              />
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <AdminFormField :label="SETTINGS_MESSAGES.maxRequestsLabel" for="admin-rate-limit-max-requests">
                <AdminInput
                  id="admin-rate-limit-max-requests"
                  v-model="rateLimitForm.max_requests"
                  type="number"
                  :min="1"
                  :max="RATE_LIMIT_MAX_REQUESTS"
                  :disabled="rateLimitLoading || rateLimitSaving"
                />
              </AdminFormField>

              <AdminFormField :label="SETTINGS_MESSAGES.windowSecondsLabel" for="admin-rate-limit-window-seconds">
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
              {{ SETTINGS_MESSAGES.reset }}
            </AdminButton>
            <AdminButton
              :class="adminActionButtonClass"
              :disabled="rateLimitLoading || rateLimitSaving"
              :loading="rateLimitSaving"
              @click="saveRateLimit"
            >
              {{ SETTINGS_MESSAGES.save }}
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
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { SETTINGS_MESSAGES } from './messages'

const settingCardBodyClass = 'flex flex-col gap-5'

const access = useAccessSettings({
  feedback: useAdminAccessSettingsFeedback(),
  confirm: useAdminConfirm(),
})

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
    title: SETTINGS_MESSAGES.registrationTitle,
    description: SETTINGS_MESSAGES.registrationDescription,
    icon: ShieldCheck,
    status: enabled ? SETTINGS_MESSAGES.registrationOpen : SETTINGS_MESSAGES.registrationClosed,
    statusSeverity: enabled ? 'success' : 'warn',
    summary: SETTINGS_MESSAGES.registrationSummary(
      enabled ? SETTINGS_MESSAGES.registrationOpen : SETTINGS_MESSAGES.registrationClosed,
      maxRequests,
      windowSeconds,
    ),
  }
}

const staticOutboundRow: AdminSettingsTableRow = {
  id: 'outbound_policy',
  title: SETTINGS_MESSAGES.outboundTitle,
  description: SETTINGS_MESSAGES.outboundDescription,
  icon: Server,
  status: SETTINGS_MESSAGES.outboundStatus,
  statusSeverity: 'secondary',
  summary: SETTINGS_MESSAGES.outboundSummary,
}

const settingRows = computed<AdminSettingsTableRow[]>(() => [
  buildRegistrationRow(registrationEnabled.value, rateLimitForm.value),
  staticOutboundRow,
])
</script>
