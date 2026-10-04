<template>
  <div
    class="flex flex-col justify-between rounded-xl border border-border bg-card p-5 text-foreground"
  >
    <div class="flex flex-col gap-1">
      <span class="text-admin-body font-medium text-muted-foreground tracking-normal">
        {{ label }}
      </span>
      <span class="text-admin-modal-title font-[780] text-foreground leading-[1.05]">
        {{ value }}
      </span>
    </div>
    <div v-if="footer !== undefined" class="mt-3 text-admin-meta text-muted-foreground">
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
