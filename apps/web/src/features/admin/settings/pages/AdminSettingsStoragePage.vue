<template>
  <div :class="adminDashboardRootClass" class="flex flex-col gap-6">
    <!-- 1. 对象存储供应商管理 (1:1 复刻功能定价页规范) -->
    <AdminStoragePage />

    <!-- 2. 上传并发设置表格 (平铺展开为标准表格，行内配置与操作) -->
    <section :aria-label="SETTINGS_MESSAGES.uploadSectionAria" class="flex flex-col gap-2">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <div class="flex items-center gap-2">
          <h3 class="text-admin-title font-bold text-admin-title-color">{{ SETTINGS_MESSAGES.uploadTitle }}</h3>
          <span class="text-admin-meta text-admin-subtitle-color"
            >{{ SETTINGS_MESSAGES.uploadSubtitle }}</span
          >
        </div>
        <span class="text-admin-meta text-admin-text-3"
          >{{ SETTINGS_MESSAGES.paramCount(settingRows.length) }}</span
        >
      </div>

      <div :class="adminTableCardClass">
        <AdminTable
          :value="settingRows"
          :loading="mediaUploadLoading"
          data-key="id"
          scrollable
          :frame-class="adminTableFrameClass"
          :table-style="adminTableStyle"
          :class="adminTableClass"
          :table-aria-label="SETTINGS_MESSAGES.uploadTableAria"
          :scroll-aria-label="SETTINGS_MESSAGES.uploadTableScrollAria"
        >
          <template #empty>
            <AdminEmptyState :title="SETTINGS_MESSAGES.emptyConfigs" />
          </template>

          <!-- 配置项列 -->
          <AdminColumn field="title" :header="SETTINGS_MESSAGES.itemColumn" :style="{ width: '280px', minWidth: '220px' }">
            <template #body="{ data }">
              <AdminItemCell
                :icon="data.icon"
                :title="data.title"
                :description="data.description"
              />
            </template>
          </AdminColumn>

          <!-- 参数键名列 -->
          <AdminColumn field="key" :header="SETTINGS_MESSAGES.keyColumn" :style="{ width: '220px', minWidth: '180px' }">
            <template #body="{ data }">
              <span
                class="inline-block rounded border border-admin-border-soft bg-admin-surface-muted px-2 py-0.5 font-mono text-admin-caption font-medium text-admin-text-2"
              >
                {{ data.key }}
              </span>
            </template>
          </AdminColumn>

          <!-- 取值范围列 -->
          <AdminColumn
            field="range"
            :header="SETTINGS_MESSAGES.rangeColumn"
            :style="{ width: '120px', minWidth: '100px' }"
          >
            <template #body="{ data }">
              <span class="font-mono text-admin-body text-admin-text-2">
                {{ data.min }} ~ {{ data.max }}
              </span>
            </template>
          </AdminColumn>

          <!-- 默认值列 -->
          <AdminColumn
            field="defaultValue"
            :header="SETTINGS_MESSAGES.defaultColumn"
            :style="{ width: '100px', minWidth: '80px' }"
          >
            <template #body="{ data }">
              <span class="font-mono text-admin-body text-admin-text-3">
                {{ data.defaultValue }}
              </span>
            </template>
          </AdminColumn>

          <!-- 并发数设置列 -->
          <AdminColumn
            field="upload_max_concurrent"
            :header="SETTINGS_MESSAGES.concurrentColumn"
            :style="{ width: '180px', minWidth: '160px' }"
          >
            <template #body>
              <div class="flex items-center gap-2">
                <AdminInput
                  id="admin-media-upload-max-concurrent"
                  v-model="mediaUploadForm.upload_max_concurrent"
                  type="number"
                  size="small"
                  :min="MEDIA_UPLOAD_MIN_CONCURRENT"
                  :max="MEDIA_UPLOAD_MAX_CONCURRENT"
                  :disabled="mediaUploadLoading || mediaUploadSaving"
                  :invalid="!!mediaUploadValidationError"
                  class="w-24 font-mono text-admin-body text-center"
                  :aria-label="SETTINGS_MESSAGES.concurrentLimitAria"
                />
                <span class="text-admin-body text-admin-text-3 select-none">{{ SETTINGS_MESSAGES.tasksUnit }}</span>
              </div>
            </template>
          </AdminColumn>

          <!-- 状态列 -->
          <AdminColumn field="status" :header="SETTINGS_MESSAGES.statusColumn" :style="{ width: '120px' }">
            <template #body>
              <AdminBadge
                :value="mediaUploadIsDirty ? SETTINGS_MESSAGES.modifiedStatus : SETTINGS_MESSAGES.activeStatus"
                :severity="mediaUploadIsDirty ? 'warn' : 'success'"
                rounded
              />
            </template>
          </AdminColumn>

          <!-- 操作列 -->
          <AdminColumn :header="SETTINGS_MESSAGES.actionsColumn" :style="{ width: '160px', minWidth: '150px' }">
            <template #body>
              <div class="flex items-center gap-2">
                <AdminButton
                  size="small"
                  :class="adminActionButtonClass"
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
                </AdminButton>
                <AdminButton
                  size="small"
                  variant="outline"
                  :class="adminActionButtonClass"
                  :disabled="mediaUploadLoading || mediaUploadSaving || !mediaUploadIsDirty"
                  @click="resetMediaUpload"
                >
                  {{ SETTINGS_MESSAGES.reset }}
                </AdminButton>
              </div>
            </template>
          </AdminColumn>
        </AdminTable>
      </div>

      <!-- 校验提示信息 -->
      <div v-if="mediaUploadValidationError" class="px-1 text-admin-caption text-admin-danger">
        {{ mediaUploadValidationError }}
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { Upload } from '@iconoir/vue'
import { computed, type Component } from 'vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminItemCell from '../../shared/components/AdminItemCell.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import AdminStoragePage from '../../storage/pages/AdminStoragePage.vue'
import {
  DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  useMediaUploadSettings,
} from '../useMediaUploadSettings'
import { useAdminMediaUploadSettingsFeedback } from '../useSettingsFeedback'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import {
  adminActionButtonClass,
  adminDashboardRootClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { SETTINGS_MESSAGES } from './messages'

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

interface SettingTableRow {
  id: 'media_upload'
  title: string
  description: string
  key: string
  icon: Component
  min: number
  max: number
  defaultValue: number
}

function buildMediaUploadRow(): SettingTableRow {
  return {
    id: 'media_upload',
    title: SETTINGS_MESSAGES.uploadRowTitle,
    description: SETTINGS_MESSAGES.uploadRowDescription,
    key: 'upload_max_concurrent',
    icon: Upload,
    min: MEDIA_UPLOAD_MIN_CONCURRENT,
    max: MEDIA_UPLOAD_MAX_CONCURRENT,
    defaultValue: DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING.upload_max_concurrent,
  }
}

const settingRows = computed<SettingTableRow[]>(() => [buildMediaUploadRow()])
</script>
