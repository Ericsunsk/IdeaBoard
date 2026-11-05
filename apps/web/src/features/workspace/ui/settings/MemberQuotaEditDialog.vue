<template>
  <AdminDialogShell
    :visible="props.isVisible"
    :header="props.mode === 'edit' ? t(editKeys.editTitle) : t(editKeys.createTitle)"
    :description="t(editKeys.description)"
    size="compact"
    content-layout="form"
    :busy="props.isPending"
    @update:visible="closeDialog"
    @close="closeDialog"
  >
    <!-- p-1/-m-1 reserves room for the Input/Select focus rings inside the
      AdminDialogShell scrollport (overflow clip for rounded corners);
      the negative margin keeps the outer layout unchanged. -->
    <form class="flex flex-col gap-4 p-1 -m-1" @submit.prevent="emit('submit')">
      <AdminFormField :label="t('workspace.quota.memberLabel')" for="member-quota-edit-member">
        <AdminSelect
          id="member-quota-edit-member"
          :model-value="props.member"
          :options="props.members"
          :disabled="props.mode === 'edit' || props.isPending"
          :placeholder="t(editKeys.memberPlaceholder)"
          @update:model-value="forwardMember"
        />
      </AdminFormField>

      <AdminFormField :label="t('workspace.quota.periodLabel')" for="member-quota-edit-period">
        <AdminSelect
          id="member-quota-edit-period"
          :model-value="props.period"
          :options="periodOptions"
          :disabled="props.isPending"
          @update:model-value="forwardPeriod"
        />
      </AdminFormField>

      <AdminFormField
        v-if="props.period !== 'unlimited'"
        :label="t('workspace.quota.limitLabel')"
        for="member-quota-edit-limit"
      >
        <AdminInput
          id="member-quota-edit-limit"
          :model-value="props.limit"
          :disabled="props.isPending"
          inputmode="numeric"
          @update:model-value="emit('update:limit', String($event ?? ''))"
          @enter="emit('submit')"
        />
      </AdminFormField>
      <p v-else class="m-0 text-admin-caption text-muted-foreground">
        {{ t(editKeys.unlimitedHint) }}
      </p>

      <p
        v-if="props.fieldError"
        class="m-0 text-admin-meta leading-body text-destructive"
        role="alert"
      >
        {{ props.fieldError }}
      </p>
    </form>

    <template #footer>
      <AdminDialogActions
        :confirm-label="t('workspace.quota.save')"
        :loading="props.isPending"
        :disabled="props.isPending"
        :cancel-disabled="props.isPending"
        @cancel="closeDialog"
        @confirm="emit('submit')"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AdminDialogActions,
  AdminDialogShell,
  AdminFormField,
  AdminInput,
  AdminSelect,
  type SelectOptionItem,
} from '@/features/admin/shared'
import {
  quotaPeriodLabelKey,
  type MemberQuotaPeriod,
} from '@/features/account/session/quota/resource'
import { MEMBER_QUOTA_EDIT_KEYS, PERIOD_VALUES, type MemberQuotaEditMode } from './quotaKeys'

// Member quota edit dialog (Org-only): create-first and per-row adjust share
// one compact form. Copy resolves through the locale bundle (four-locale
// parity); field labels stay on existing workspace.quota keys.

const props = defineProps<{
  isVisible: boolean
  mode: MemberQuotaEditMode
  members: readonly SelectOptionItem[]
  member: string
  period: MemberQuotaPeriod
  limit: string
  isPending: boolean
  fieldError: string | null
}>()

const emit = defineEmits<{
  'update:isVisible': [isVisible: boolean]
  'update:member': [value: unknown]
  'update:period': [value: unknown]
  'update:limit': [value: string]
  submit: []
}>()

const MEMBER_SELECT_ID = 'member-quota-edit-member'

const { t } = useI18n()

const editKeys = MEMBER_QUOTA_EDIT_KEYS

const periodOptions = computed(() =>
  PERIOD_VALUES.map((period) => ({ label: t(quotaPeriodLabelKey(period)), value: period })),
)

function closeDialog(): void {
  emit('update:isVisible', false)
}

function forwardMember(value: unknown): void {
  emit('update:member', value)
}

function forwardPeriod(value: unknown): void {
  emit('update:period', value)
}

// Empty-state create flow focuses the member picker once the dialog opens.
watch(
  () => props.isVisible,
  (isNowVisible) => {
    if (isNowVisible && props.mode === 'create') {
      nextTick(() => {
        document.getElementById(MEMBER_SELECT_ID)?.focus()
      })
    }
  },
)
</script>
