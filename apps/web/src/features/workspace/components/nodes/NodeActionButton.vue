<template>
  <button
    type="button"
    class="inline-flex items-center justify-center border text-[11px] leading-none whitespace-nowrap transition-all duration-200 ease-[ease] enabled:cursor-pointer disabled:cursor-not-allowed disabled:opacity-45"
    :class="buttonClasses"
    :disabled="disabled"
    @click="emit('click', $event)"
  >
    <slot />
  </button>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    variant?: 'stage' | 'stage-ghost' | 'tool' | 'chip' | 'primary'
    active?: boolean
    disabled?: boolean
    full?: boolean
    pushRight?: boolean
  }>(),
  {
    variant: 'tool',
    active: false,
    disabled: false,
    full: false,
    pushRight: false,
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

type ButtonVariant = NonNullable<(typeof props)['variant']>

function getVariantClasses(variant: ButtonVariant, active: boolean) {
  if (variant === 'stage') {
    return 'min-h-[26px] rounded-lg border-none bg-[rgba(17,24,39,0.74)] px-[10px] font-medium text-[#f9fafb] enabled:hover:bg-[rgba(17,24,39,0.9)]'
  }
  if (variant === 'stage-ghost') {
    return 'min-h-[26px] rounded-lg border-none bg-[rgba(75,85,99,0.72)] px-[10px] font-medium text-[#f9fafb] enabled:hover:bg-[rgba(75,85,99,0.88)]'
  }
  if (variant === 'primary') {
    return 'min-h-[34px] rounded-[10px] border-[#9278e6] bg-[#9278e6] px-3 text-xs font-bold tracking-[0.01em] text-white enabled:hover:border-[#7f68db] enabled:hover:bg-[#7f68db]'
  }

  const activeColors = 'border-[rgba(122,107,213,0.4)] bg-[rgba(122,107,213,0.12)] text-[#6757c8]'
  const baseColors =
    'border-[#d6dcec] bg-white text-[#5f6877] enabled:hover:border-[#c4b7f4] enabled:hover:text-[#6757c8]'
  const colors = active ? activeColors : baseColors

  if (variant === 'chip') {
    return `h-6 min-h-6 rounded-lg px-2 text-[10px] font-bold tracking-[0.02em] ${colors}`
  }

  return `min-h-7 rounded-lg px-[9px] font-semibold ${colors}`
}

const buttonClasses = computed(() => [
  getVariantClasses(props.variant, props.active),
  props.full && 'w-full',
  props.pushRight && 'ml-auto',
])
</script>
