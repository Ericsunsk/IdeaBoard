<template>
  <div class="flex w-full flex-wrap items-center gap-x-3 gap-y-2">
    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
      <span :id="LABEL_ID" class="shrink-0 text-admin-body font-medium text-foreground">
        {{ SETTINGS_MESSAGES.uploadRowTitle }}
      </span>
      <AdminInput
        id="admin-media-upload-max-concurrent"
        v-model="mediaUploadForm.upload_max_concurrent"
        type="number"
        :min="MEDIA_UPLOAD_MIN_CONCURRENT"
        :max="MEDIA_UPLOAD_MAX_CONCURRENT"
        :disabled="mediaUploadLoading || mediaUploadSaving"
        :invalid="!!mediaUploadValidationError"
        :aria-labelledby="LABEL_ID"
        class="w-20 font-mono text-center text-admin-body"
      />
      <span class="shrink-0 text-admin-body text-muted-foreground select-none">
        {{ SETTINGS_MESSAGES.tasksUnit }}
      </span>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <ActionButton
        size="small"
        :disabled="mediaUploadLoading || mediaUploadSaving || !mediaUploadIsDirty"
        @click="resetMediaUpload"
      >
        {{ SETTINGS_MESSAGES.reset }}
      </ActionButton>
      <ActionButton
        size="small"
        :disabled="
          mediaUploadLoading ||
          mediaUploadSaving ||
          !mediaUploadIsDirty ||
          !!mediaUploadValidationError
        "
        :loading="mediaUploadSaving"
        @click="saveMediaUpload"
      >
        {{ SETTINGS_MESSAGES.save }}
      </ActionButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import ActionButton from '../../../../core/components/ActionButton.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import { SETTINGS_MESSAGES } from '../pages/messages'
import {
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  useMediaUploadSettings,
} from './useMediaUploadSettings'
import { useAdminMediaUploadSettingsFeedback } from '../shared/useSettingsFeedback'

const LABEL_ID = 'admin-media-upload-footer-label'

const mediaUpload = useMediaUploadSettings({
  feedback: useAdminMediaUploadSettingsFeedback(),
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
