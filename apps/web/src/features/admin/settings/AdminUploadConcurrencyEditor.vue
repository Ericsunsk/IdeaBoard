<template>
  <SettingEditorCard
    :icon="Upload"
    :title="SETTINGS_MESSAGES.uploadTitle"
    :description="SETTINGS_MESSAGES.uploadSubtitle"
    plain
  >
    <div class="flex flex-col gap-4">
      <!-- 校验提示信息 -->
      <div v-if="mediaUploadValidationError" class="text-admin-caption text-destructive">
        {{ mediaUploadValidationError }}
      </div>

      <!-- 并发设置项卡片行 -->
      <div
        class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between rounded-lg border border-border bg-muted/40 p-4 transition-colors"
      >
        <div class="flex items-start gap-3.5 min-w-0">
          <div
            class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground shadow-xs"
          >
            <Upload class="size-4.5" />
          </div>
          <div class="flex min-w-0 flex-col gap-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-admin-body font-bold text-foreground">
                {{ SETTINGS_MESSAGES.uploadRowTitle }}
              </span>
              <span
                class="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-admin-caption font-medium text-muted-foreground"
              >
                upload_max_concurrent
              </span>
            </div>
            <p class="text-admin-caption text-muted-foreground">
              {{ SETTINGS_MESSAGES.uploadRowDescription }}（取值范围
              {{ MEDIA_UPLOAD_MIN_CONCURRENT }} ~ {{ MEDIA_UPLOAD_MAX_CONCURRENT }}，默认值
              {{ DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING.upload_max_concurrent }}）
            </p>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-2 self-start sm:self-center">
          <AdminInput
            id="admin-media-upload-max-concurrent"
            v-model="mediaUploadForm.upload_max_concurrent"
            type="number"
            :min="MEDIA_UPLOAD_MIN_CONCURRENT"
            :max="MEDIA_UPLOAD_MAX_CONCURRENT"
            :disabled="mediaUploadLoading || mediaUploadSaving"
            :invalid="!!mediaUploadValidationError"
            class="w-24 font-mono text-center text-admin-body"
            :aria-label="SETTINGS_MESSAGES.concurrentLimitAria"
          />
          <span class="text-admin-body text-muted-foreground select-none">
            {{ SETTINGS_MESSAGES.tasksUnit }}
          </span>
        </div>
      </div>
    </div>

    <template #actions>
      <AdminEditorActions
        :save-disabled="
          mediaUploadLoading ||
          mediaUploadSaving ||
          !mediaUploadIsDirty ||
          !!mediaUploadValidationError
        "
        :reset-disabled="mediaUploadLoading || mediaUploadSaving || !mediaUploadIsDirty"
        :saving="mediaUploadSaving"
        @save="saveMediaUpload"
        @reset="resetMediaUpload"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { Upload } from '@iconoir/vue'
import AdminEditorActions from '../shared/components/AdminEditorActions.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import SettingEditorCard from './SettingEditorCard.vue'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { SETTINGS_MESSAGES } from './pages/messages'
import {
  DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  useMediaUploadSettings,
} from './useMediaUploadSettings'
import { useAdminMediaUploadSettingsFeedback } from './useSettingsFeedback'

const mediaUpload = useMediaUploadSettings({
  feedback: useAdminMediaUploadSettingsFeedback(),
  confirm: useAdminConfirm(),
})

const {
  form: mediaUploadForm,
  isDirty: mediaUploadIsDirty,
  loading: mediaUploadLoading,
  saving: mediaUploadSaving,
  validationError: mediaUploadValidationError,
  reset: resetMediaUpload,
  save: saveMediaUpload,
} = mediaUpload
</script>
