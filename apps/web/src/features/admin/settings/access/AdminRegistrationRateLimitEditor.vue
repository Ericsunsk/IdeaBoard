<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.ratelimit.icon"
    :title="SETTINGS_SECTIONS.ratelimit.title"
    :description="SETTINGS_MESSAGES.registrationDescription"
    plain
  >
    <div class="flex flex-col gap-4">
      <div
        class="flex min-h-12 items-center justify-between gap-4 rounded-lg border border-border bg-muted/50 px-4 py-3"
      >
        <div class="flex flex-col">
          <span class="text-admin-body font-bold text-foreground">
            {{ SETTINGS_MESSAGES.registrationOpen }}
          </span>
          <span class="text-admin-caption text-muted-foreground">
            {{ SETTINGS_MESSAGES.registrationHint }}
          </span>
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
        <AdminFormField
          :label="SETTINGS_MESSAGES.maxRequestsLabel"
          for="admin-rate-limit-max-requests"
        >
          <AdminInput
            id="admin-rate-limit-max-requests"
            v-model="rateLimitForm.max_requests"
            type="number"
            :min="1"
            :max="RATE_LIMIT_MAX_REQUESTS"
            :disabled="rateLimitLoading || rateLimitSaving"
          />
        </AdminFormField>

        <AdminFormField
          :label="SETTINGS_MESSAGES.windowSecondsLabel"
          for="admin-rate-limit-window-seconds"
        >
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
      <AdminEditorActions
        :save-disabled="rateLimitLoading || rateLimitSaving"
        :reset-disabled="rateLimitLoading || rateLimitSaving"
        :saving="rateLimitSaving"
        @save="saveRateLimit"
        @reset="resetRateLimit"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import AdminEditorActions from '../../shared/components/AdminEditorActions.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import SettingEditorCard from '../shared/SettingEditorCard.vue'
import { SETTINGS_SECTIONS } from '../shared/sections'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  useAccessSettings,
} from './useAccessSettings'
import { useAdminAccessSettingsFeedback } from '../shared/useSettingsFeedback'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { SETTINGS_MESSAGES } from '../pages/messages'

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
</script>
