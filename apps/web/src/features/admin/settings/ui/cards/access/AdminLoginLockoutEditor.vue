<template>
  <SettingEditorCard
    :icon="ShieldCheck"
    :title="SETTINGS_MESSAGES.loginLockoutSectionAria"
    :description="LOGIN_LOCKOUT_DESCRIPTION"
    plain
  >
    <div class="grid gap-4 md:grid-cols-2">
      <AdminFormField
        :label="SETTINGS_MESSAGES.loginLockoutThresholdLabel"
        for="admin-login-lockout-threshold"
        :hint="SETTINGS_MESSAGES.loginLockoutThresholdHint"
      >
        <AdminInput
          id="admin-login-lockout-threshold"
          v-model="loginLockoutForm.threshold"
          type="number"
          :min="LOGIN_LOCKOUT_MIN_THRESHOLD"
          :max="LOGIN_LOCKOUT_MAX_THRESHOLD"
          :disabled="loginLockoutLoading || loginLockoutSaving"
        />
      </AdminFormField>

      <AdminFormField
        :label="SETTINGS_MESSAGES.loginLockoutDurationLabel"
        for="admin-login-lockout-duration"
        :hint="SETTINGS_MESSAGES.loginLockoutDurationHint"
      >
        <AdminInput
          id="admin-login-lockout-duration"
          v-model="loginLockoutForm.duration_seconds"
          type="number"
          :min="LOGIN_LOCKOUT_MIN_DURATION_SECONDS"
          :max="LOGIN_LOCKOUT_MAX_DURATION_SECONDS"
          :disabled="loginLockoutLoading || loginLockoutSaving"
        />
      </AdminFormField>
    </div>
    <template #actions>
      <AdminEditorActions
        :save-disabled="loginLockoutLoading || loginLockoutSaving"
        :reset-disabled="loginLockoutLoading || loginLockoutSaving"
        :saving="loginLockoutSaving"
        @save="saveLoginLockout"
        @reset="resetLoginLockout"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { ShieldCheck } from '@iconoir/vue'
import { AdminEditorActions } from '../../../../shared/index'
import { AdminFormField } from '../../../../shared/index'
import { AdminInput } from '../../../../shared/index'
import SettingEditorCard from '../SettingEditorCard.vue'
import {
  LOGIN_LOCKOUT_MAX_DURATION_SECONDS,
  LOGIN_LOCKOUT_MAX_THRESHOLD,
  LOGIN_LOCKOUT_MIN_DURATION_SECONDS,
  LOGIN_LOCKOUT_MIN_THRESHOLD,
  useAccessSettings,
} from '../../../session/access/useAccessSettings'
import { useAdminAccessSettingsFeedback } from '../../../session/useSettingsFeedback'
import { SETTINGS_MESSAGES } from '../../pages/messages'

// 独立登录锁定卡：复用 useAccessSettings 内 loginLockout 分支（query/mutation/
// lifecycle/toast 全收口在 hook），与注册限流卡平级挂在统一设置页同一 section。
const LOGIN_LOCKOUT_DESCRIPTION = '连续登录失败达到阈值后锁定账号，到期自动解锁'

const access = useAccessSettings({
  feedback: useAdminAccessSettingsFeedback(),
})

const {
  loginLockoutForm,
  loginLockoutLoading,
  loginLockoutSaving,
  resetLoginLockout,
  saveLoginLockout,
} = access
</script>
