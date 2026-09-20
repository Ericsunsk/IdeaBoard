<template>
  <button
    type="button"
    class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer disabled:opacity-50"
    :disabled="props.disabled || props.loading"
    :aria-label="props.ariaLabel"
    @click="emit('click')"
  >
    <Refresh class="size-3.5 text-admin-text-2" :class="props.loading ? 'animate-spin' : ''" />
    <span>{{ props.loading ? '同步中...' : '刷新' }}</span>
  </button>
</template>

<script setup lang="ts">
import { Refresh } from '@iconoir/vue'

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
