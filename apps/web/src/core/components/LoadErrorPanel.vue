<template>
  <div role="alert" :class="[panelClass, alignClass]">
    <p class="m-0 text-sm font-semibold text-foreground">{{ props.message }}</p>
    <p v-if="props.detail" class="m-0 text-sm text-muted-foreground">{{ props.detail }}</p>
    <Button
      type="button"
      :variant="props.retryVariant"
      size="sm"
      :is-loading="props.isRetrying"
      @click="emit('retry')"
    >
      {{ props.retryLabel }}
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'

// Shared load-error panel: one destructive panel (border-destructive/30 on
// bg-destructive/10) with a small retry button for every load-failure block.
// Table empty states keep the ghost retry treatment through `retryVariant`.
type LoadErrorPanelAlign = 'start' | 'center'
type LoadErrorPanelRetryVariant = 'default' | 'ghost'

const props = withDefaults(
  defineProps<{
    message: string
    detail?: string | undefined
    retryLabel: string
    isRetrying?: boolean | undefined
    align?: LoadErrorPanelAlign | undefined
    retryVariant?: LoadErrorPanelRetryVariant | undefined
  }>(),
  {
    detail: undefined,
    isRetrying: false,
    align: 'start',
    retryVariant: 'default',
  },
)

const emit = defineEmits<{
  retry: []
}>()

const panelClass =
  'flex flex-col gap-2 rounded-panel border border-destructive/30 bg-destructive/10 p-4 text-sm text-foreground'

const alignClass = computed(() =>
  props.align === 'center' ? 'items-center text-center' : 'items-start text-left',
)
</script>
