<template>
  <div
    :class="
      cn(
        'rounded-panel border border-border bg-card text-card-foreground',
        elevated && 'shadow',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class']
    /**
     * Elevation is an explicit contract rather than a class to cancel out.
     * A baked-in `shadow` forces every flat surface to override it with
     * `shadow-none`, which only wins by accident of `cn()` class ordering and
     * silently diverges the moment a wrapper forgets to pass the override.
     * Surfaces own their own level: pass `false` to opt out.
     *
     * `undefined` means "keep the default", so forwarding wrappers can pass
     * their own optional prop straight through under exactOptionalPropertyTypes.
     */
    elevated?: boolean | undefined
  }>(),
  {
    class: undefined,
    elevated: true,
  },
)
</script>
