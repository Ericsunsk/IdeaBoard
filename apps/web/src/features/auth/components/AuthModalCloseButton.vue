<template>
  <Button
    unstyled
    type="button"
    :class="authCloseButtonClass"
    :disabled="disabled"
    :aria-label="t('common.actions.close')"
    @click="emit('click')"
  >
    <template #icon>
      <Xmark class="size-5" aria-hidden="true" />
    </template>
  </Button>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { authCloseButtonClass } from './authModalUi'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
</script>
