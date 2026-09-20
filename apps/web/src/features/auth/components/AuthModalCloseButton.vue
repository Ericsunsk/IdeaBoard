<template>
  <button
    type="button"
    :class="authCloseButtonClass"
    :disabled="disabled"
    :aria-label="t('common.actions.close')"
    @click="emit('click')"
  >
    <Xmark class="size-5" aria-hidden="true" />
  </button>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { useI18n } from 'vue-i18n'
import { authCloseButtonClass } from './modalUi'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
</script>
