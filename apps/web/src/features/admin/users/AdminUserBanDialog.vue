<template>
  <AdminDialogShell
    :visible="visible"
    aria-label="封禁用户"
    description="设置封禁时长与原因后确认封禁。"
    size="compact"
    :busy="loading"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div :class="[adminDialogHeaderContentClass, 'pr-4']">
        <div class="mb-3 flex flex-wrap gap-2">
          <AdminBadge severity="secondary">访问控制</AdminBadge>
          <AdminBadge severity="danger">强制限制</AdminBadge>
        </div>
        <h2 :class="adminDialogTitleClass">高级访问限制</h2>
        <p class="mt-2.5 mb-0 text-admin-content leading-7 text-admin-subtitle-color">
          {{ description }}
        </p>
      </div>
    </template>

    <label
      for="admin-ban-duration-select"
      class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
    >
      <span>封禁时长 (Duration)</span>
      <AdminSelect
        id="admin-ban-duration-select"
        :model-value="expiresInDays"
        :options="durationOptions"
        class="w-full"
        @update:model-value="handleExpiresInDaysUpdate"
      />
    </label>
    <label
      for="admin-ban-reason-textarea"
      class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
    >
      <span>封禁原因 (内部记录)</span>
      <AdminTextarea
        id="admin-ban-reason-textarea"
        :model-value="reason"
        placeholder="例如：发表违规内容、恶意刷量..."
        :rows="3"
        class="w-full"
        @update:model-value="handleReasonUpdate"
      />
    </label>

    <template #footer>
      <div class="flex w-full flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div class="max-w-[480px] text-admin-meta leading-6 text-admin-text-3">
          封禁会立即断开该用户所有连接，封禁期间无法重新登录。
        </div>
        <div :class="adminDialogFooterActionsClass">
          <AdminButton
            variant="outline"
            :class="adminActionButtonClass"
            :disabled="loading"
            @click="$emit('cancel')"
          >
            取消
          </AdminButton>
          <AdminButton
            variant="destructive"
            :class="adminActionButtonClass"
            :loading="loading"
            @click="$emit('confirm')"
          >
            确认封禁
          </AdminButton>
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import AdminBadge from '../shared/components/AdminBadge.vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminSelect from '../shared/components/AdminSelect.vue'
import AdminTextarea from '../shared/components/AdminTextarea.vue'
import {
  adminDialogFooterActionsClass,
  adminDialogHeaderContentClass,
  adminDialogTitleClass,
  adminActionButtonClass,
} from '../shared/lib/uiClasses'

type BanDurationOption = {
  label: string
  value: number
}

withDefaults(
  defineProps<{
    visible: boolean
    description: string
    expiresInDays: number
    reason: string
    durationOptions: BanDurationOption[]
    loading?: boolean
  }>(),
  {
    loading: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  'update:expiresInDays': [expiresInDays: number]
  'update:reason': [reason: string]
  cancel: []
  confirm: []
}>()

function handleVisibleUpdate(visible: boolean) {
  emit('update:visible', visible)
}

function handleExpiresInDaysUpdate(value: number | null | undefined) {
  emit('update:expiresInDays', value ?? 0)
}

function handleReasonUpdate(value: string | undefined) {
  emit('update:reason', value ?? '')
}
</script>
