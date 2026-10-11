<template>
  <SwitchRoot
    :model-value="modelValue"
    :disabled="disabled"
    :class="
      cn(
        // The 20px track is the design; `hit-target-32` grows the hit area to
        // the repo floor without inflating it. Without this the switch is the
        // only target in most rows and fell under WCAG 2.2 AA 2.5.8.
        // Transition: `control-state` supplies the --motion-control-* duration
        // and easing (never stack a transition/duration/ease utility on top:
        // its utilities-layer default would shadow the token); the property
        // list stays explicit and includes opacity so disabled fades smoothly.
        'hit-target-32 peer control-state inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent [transition-property:background-color,border-color,color,opacity] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input',
        props.class,
      )
    "
    @update:model-value="(val: boolean) => emit('update:modelValue', val)"
  >
    <SwitchThumb
      :class="
        cn(
          'pointer-events-none block size-4 rounded-full bg-background shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0',
        )
      "
    />
  </SwitchRoot>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    class?: HTMLAttributes['class']
    disabled?: boolean
  }>(),
  {
    modelValue: false,
    class: undefined,
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()
</script>
