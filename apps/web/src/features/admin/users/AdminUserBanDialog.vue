<template>
  <AdminDialogShell
    :visible="visible"
    aria-label="封禁用户"
    size="compact"
    :busy="loading"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div :class="[adminDialogHeaderContentClass, 'pr-4']">
        <div class="mb-3 flex flex-wrap gap-2">
          <Tag value="访问控制" severity="secondary" rounded />
          <Tag value="强制限制" severity="danger" rounded />
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
      <Select
        input-id="admin-ban-duration-select"
        :model-value="expiresInDays"
        name="ban-duration"
        :options="durationOptions"
        option-label="label"
        option-value="value"
        class="w-full"
        @update:model-value="handleExpiresInDaysUpdate"
      />
    </label>
    <label
      for="admin-ban-reason-textarea"
      class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
    >
      <span>封禁原因 (内部记录)</span>
      <Textarea
        id="admin-ban-reason-textarea"
        :model-value="reason"
        name="ban-reason"
        placeholder="例如：发表违规内容、恶意刷量..."
        rows="3"
        auto-resize
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
          <Button
            label="取消"
            :class="adminActionButtonClass"
            severity="secondary"
            outlined
            :disabled="loading"
            @click="$emit('cancel')"
          />
          <Button
            label="确认封禁"
            :class="adminActionButtonClass"
            severity="danger"
            :loading="loading"
            @click="$emit('confirm')"
          />
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import {
  adminDialogFooterActionsClass,
  adminDialogHeaderContentClass,
  adminDialogTitleClass,
  adminActionButtonClass,
} from '../shared/lib/adminUiClasses'

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
