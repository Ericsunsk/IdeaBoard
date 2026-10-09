<template>
  <span
    v-if="src || fallbackText"
    class="inline-flex shrink-0 items-center justify-center overflow-hidden text-current"
  >
    <img v-if="src" :src="src" alt="" :class="imageClass || 'h-full w-full object-cover'" />
    <span v-else :class="fallbackClass">{{ fallbackText }}</span>
  </span>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    fallbackClass?: string
    fallbackText?: string
    imageClass?: string
    src?: string
  }>(),
  {
    fallbackClass: 'select-none text-center text-xs leading-none font-[850] tracking-normal',
    fallbackText: '',
    imageClass: '',
    src: '',
  },
)
</script>
