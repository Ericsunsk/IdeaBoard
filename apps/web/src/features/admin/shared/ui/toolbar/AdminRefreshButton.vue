<template>
  <button
    type="button"
    class="flex h-9 items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted cursor-pointer disabled:opacity-50"
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
