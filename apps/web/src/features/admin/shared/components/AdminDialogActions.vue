<template>
  <ActionButton variant="outline" :disabled="cancelDisabled" @click="emit('cancel')">
    {{ cancelLabel }}
  </ActionButton>
  <ActionButton
    :variant="confirmVariant"
    :severity="severity"
    :loading="loading"
    :disabled="disabled"
    @click="emit('confirm')"
  >
    {{ confirmLabel }}
  </ActionButton>
</template>

<script setup lang="ts">
import ActionButton from '@/shared/components/ActionButton.vue'
import type { AdminButtonVariant } from './AdminButton.vue'

withDefaults(
  defineProps<{
    cancelLabel: string
    confirmLabel: string
    loading?: boolean
    disabled?: boolean
    cancelDisabled?: boolean
    confirmVariant?: AdminButtonVariant
    severity?: 'primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn' | undefined
  }>(),
  {
    loading: false,
    disabled: false,
    cancelDisabled: false,
    confirmVariant: 'default',
    severity: undefined,
  },
)

const emit = defineEmits<{
  cancel: []
  confirm: []
}>()
</script>
