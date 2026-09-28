<template>
  <AdminDialogShell
    :visible="visible"
    :header="title"
    :description="CREDITS_SHARED_MESSAGES.actionReasonDescription"
    size="compact"
    :busy="loading"
    @reset="resetTransientState"
    @update:visible="emit('update:visible', $event)"
  >
    <AdminAlert :severity="intent === 'danger' ? 'warn' : 'info'">
      {{ message }}
    </AdminAlert>

    <AdminActionReasonField
      v-model="reason"
      input-id="admin-action-reason"
      :error="showError ? validation.error : ''"
      :rows="4"
      @update:model-value="showError = false"
    />

    <template #footer="{ requestClose }">
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="loading"
        @click="requestClose"
      >
        {{ CREDITS_SHARED_MESSAGES.actionReasonCancel }}
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :severity="intent"
        :loading="loading"
        @click="submit"
      >
        {{ confirmLabel }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { validateAdminActionReason } from '../../shared/validation/actionReasonContract'
import AdminActionReasonField from './AdminActionReasonField.vue'
import { CREDITS_SHARED_MESSAGES } from './messages'

withDefaults(
  defineProps<{
    visible: boolean
    title: string
    message: string
    confirmLabel: string
    intent?: 'warn' | 'danger'
    loading?: boolean
  }>(),
  {
    intent: 'warn',
    loading: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  confirm: [reason: string]
}>()

const reason = ref('')
const showError = ref(false)
const validation = computed(() => validateAdminActionReason(reason.value))

function resetTransientState() {
  reason.value = ''
  showError.value = false
}

function submit() {
  showError.value = true
  if (!validation.value.valid) return
  emit('confirm', validation.value.normalizedReason)
}
</script>
