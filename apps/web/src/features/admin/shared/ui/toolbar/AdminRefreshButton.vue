<template>
  <button
    type="button"
    :class="[adminToolbarButtonClass, 'disabled:opacity-50']"
    :disabled="props.disabled || props.loading"
    :aria-label="props.ariaLabel"
    @click="emit('click')"
  >
    <Refresh class="size-3.5 text-muted-foreground" :class="props.loading ? 'animate-spin' : ''" />
    <span>{{ props.loading ? '同步中...' : '刷新' }}</span>
  </button>
</template>

<script setup lang="ts">
import { Refresh } from '@iconoir/vue'
import { adminToolbarButtonClass } from '../styling/uiClasses'

const props = withDefaults(
  defineProps<{
    loading?: boolean
    disabled?: boolean
    ariaLabel?: string
  }>(),
  { loading: false, disabled: false, ariaLabel: '' },
)

const emit = defineEmits<{
  click: []
}>()
</script>
