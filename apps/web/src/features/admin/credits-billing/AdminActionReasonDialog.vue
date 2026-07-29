<template>
  <AdminDialogShell
    :visible="visible"
    :header="title"
    size="compact"
    :busy="loading"
    @reset="resetTransientState"
    @update:visible="emit('update:visible', $event)"
  >
    <Message :severity="intent === 'danger' ? 'warn' : 'info'" :closable="false">
      {{ message }}
    </Message>

    <AdminActionReasonField
      v-model="reason"
      input-id="admin-action-reason"
      :error="showError ? validation.error : ''"
      :rows="4"
      @update:model-value="showError = false"
    />

    <template #footer="{ requestClose }">
      <Button
        label="取消"
        severity="secondary"
        outlined
        :disabled="loading"
        @click="requestClose"
      />
      <Button :label="confirmLabel" :severity="intent" :loading="loading" @click="submit" />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import { validateAdminActionReason } from '../shared/validation/adminActionReasonContract'
import AdminActionReasonField from './AdminActionReasonField.vue'

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
