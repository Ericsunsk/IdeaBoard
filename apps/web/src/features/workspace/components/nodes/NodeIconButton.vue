<template>
  <button
    type="button"
    class="flex items-center justify-center rounded-lg border bg-white p-0 transition-all duration-200 ease-[ease] enabled:cursor-pointer disabled:cursor-not-allowed disabled:opacity-45"
    :class="buttonClasses"
    :style="{ width: `${size}px`, height: `${size}px` }"
    :disabled="disabled"
    :title="title"
    @click="emit('click', $event)"
  >
    <slot />
  </button>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    tone?: 'tool' | 'accent'
    active?: boolean
    disabled?: boolean
    size?: number
    title?: string
  }>(),
  {
    tone: 'tool',
    active: false,
    disabled: false,
    size: 28,
    title: '',
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

type IconTone = NonNullable<(typeof props)['tone']>

function getToneClasses(tone: IconTone, active: boolean) {
  if (tone === 'accent') {
    return 'border-[#d6dcec] text-[#6f61c6] enabled:hover:border-[#b9a8f0] enabled:hover:bg-[#f5f2ff] enabled:hover:text-[#5f4fd0]'
  }
  if (active) {
    return 'border-[rgba(122,107,213,0.4)] bg-[rgba(122,107,213,0.12)] text-[#6757c8]'
  }
  return 'border-[#d6dcec] text-[#5f6877] enabled:hover:border-[#c4b7f4] enabled:hover:text-[#6757c8]'
}

const buttonClasses = computed(() => getToneClasses(props.tone, props.active))
</script>
