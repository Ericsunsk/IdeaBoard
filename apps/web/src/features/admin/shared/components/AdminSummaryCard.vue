<template>
  <div
    class="flex flex-col justify-between rounded-xl border border-admin-border-soft bg-admin-card p-5 text-admin-text-1"
  >
    <div class="flex flex-col gap-1">
      <span class="text-admin-body font-medium text-admin-text-2 tracking-normal">
        {{ label }}
      </span>
      <span class="text-admin-modal-title font-[780] text-admin-title-color leading-[1.05]">
        {{ value }}
      </span>
    </div>
    <div v-if="footer !== undefined" class="mt-3 text-admin-meta text-admin-text-2">
      {{ footer }}
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: string | number
  footer?: string | number
}>()
</script>
