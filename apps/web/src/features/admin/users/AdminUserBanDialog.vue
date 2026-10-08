<template>
  <AdminDialogShell
    :visible="visible"
    :aria-label="USERS_MESSAGES.banAria"
    :description="USERS_MESSAGES.banDescription"
    size="compact"
    :busy="loading"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div :class="[adminDialogHeaderContentClass, 'pr-4']">
        <div class="mb-3 flex flex-wrap gap-2">
          <AdminBadge severity="secondary">{{ USERS_MESSAGES.banAccessBadge }}</AdminBadge>
          <AdminBadge severity="danger">{{ USERS_MESSAGES.banForceBadge }}</AdminBadge>
        </div>
        <h2 :class="adminDialogTitleClass">{{ USERS_MESSAGES.banTitle }}</h2>
        <p class="mt-2.5 mb-0 text-admin-content leading-7 text-muted-foreground">
          {{ description }}
        </p>
      </div>
    </template>

    <label
      for="admin-ban-duration-select"
      class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-muted-foreground"
    >
      <span>{{ USERS_MESSAGES.banDurationLabel }}</span>
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
      class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-muted-foreground"
    >
      <span>{{ USERS_MESSAGES.banReasonLabel }}</span>
      <AdminTextarea
        id="admin-ban-reason-textarea"
        :model-value="reason"
        :placeholder="USERS_MESSAGES.banReasonPlaceholder"
        :rows="3"
        class="w-full"
        @update:model-value="handleReasonUpdate"
      />
    </label>

    <template #footer>
      <div class="flex w-full flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div class="max-w-[480px] text-admin-meta leading-6 text-muted-foreground">
          {{ USERS_MESSAGES.banFooterNote }}
        </div>
        <div :class="adminDialogFooterActionsClass">
          <AdminDialogActions
            :cancel-label="USERS_MESSAGES.cancel"
            :confirm-label="USERS_MESSAGES.banConfirm"
            :loading="loading"
            :cancel-disabled="loading"
            confirm-variant="destructive"
            @cancel="$emit('cancel')"
            @confirm="$emit('confirm')"
          />
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import AdminBadge from '../shared/components/AdminBadge.vue'
import AdminDialogActions from '../shared/components/AdminDialogActions.vue'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminSelect from '../shared/components/AdminSelect.vue'
import AdminTextarea from '../shared/components/AdminTextarea.vue'
import {
  adminDialogFooterActionsClass,
  adminDialogHeaderContentClass,
  adminDialogTitleClass,
} from '../shared/lib/uiClasses'
import { USERS_MESSAGES } from './messages'

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
