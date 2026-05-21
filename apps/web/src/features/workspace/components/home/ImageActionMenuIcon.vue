<template>
  <span class="action-logo">
    <span v-if="icon === 'hd-text'" class="action-logo--text">HD</span>
    <svg
      v-else-if="icon === 'remove-bg'"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M4 5h10v10H4z" />
      <path d="M10 9h10v10H10z" />
    </svg>
    <svg
      v-else-if="icon === 'style-variation'"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path
        d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M5.6 18.4l2.8-2.8M15.6 8.4l2.8-2.8"
      />
    </svg>
    <svg
      v-else-if="icon === 'sketch-to-image'"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M18 2l-6.3 6.3M12 14l-4 4" />
      <path d="M3 21l3.5-3.5" />
      <path d="M15 6l4 4" />
      <path d="M20 7l-2-2" />
    </svg>
    <svg
      v-else-if="icon === 'image-extend'"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M9 4H5a1 1 0 0 0-1 1v4" />
      <path d="M15 4h4a1 1 0 0 1 1 1v4" />
      <path d="M9 20H5a1 1 0 0 1-1-1v-4" />
      <path d="M15 20h4a1 1 0 0 0 1-1v-4" />
      <path d="M8 12h8" />
      <path d="M12 8v8" />
    </svg>
    <svg
      v-else
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="12" cy="12" r="8" />
      <path d="M12 4v16" />
      <path d="M4 12h16" />
      <path d="M7.5 7.5l9 9" />
      <path d="M16.5 7.5l-9 9" />
    </svg>
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../config/imageActionMenu'

defineProps<{
  icon: ImageActionMenuIcon
}>()
</script>

<style scoped>
.action-logo {
  display: flex;
  align-items: center;
  justify-content: center;
}

.action-logo svg {
  width: 21px;
  height: 21px;
}

.action-logo--text {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1;
}
</style>
