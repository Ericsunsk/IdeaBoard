<template>
  <svg
    v-if="variant === 'fusion'"
    class="node-empty-state-icon node-empty-state-icon--fusion"
    width="24"
    height="24"
    viewBox="0 0 36 36"
    fill="none"
    stroke="currentColor"
    stroke-width="0.85"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path d="M3.6 3.6l4.8 4.8" />
    <path d="M8.4 8.4l-3-.7" />
    <path d="M8.4 8.4l-.7-3" />
    <path d="M32.4 3.6l-4.8 4.8" />
    <path d="M27.6 8.4l3-.7" />
    <path d="M27.6 8.4l.7-3" />
    <path d="M3.6 32.4l4.8-4.8" />
    <path d="M8.4 27.6l-3 .7" />
    <path d="M8.4 27.6l-.7 3" />
    <path d="M32.4 32.4l-4.8-4.8" />
    <path d="M27.6 27.6l3 .7" />
    <path d="M27.6 27.6l.7 3" />
    <rect x="10" y="10" width="16" height="16" rx="3" />
    <circle cx="21.5" cy="14.5" r="1.5" />
    <path d="M11 22l4.5-4.5 4 4 2-2 3.5 3.5" />
  </svg>
  <svg
    v-else
    class="node-empty-state-icon node-empty-state-icon--image"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <rect x="4" y="4" width="16" height="16" rx="3" />
    <circle cx="15.5" cy="8.5" r="1.5" />
    <path d="M5 16l4.5-4.5 4 4 2-2L19 17" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'image' | 'fusion'
  }>(),
  {
    variant: 'image',
  },
)
</script>
