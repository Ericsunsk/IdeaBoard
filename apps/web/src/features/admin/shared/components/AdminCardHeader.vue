<template>
  <div class="flex items-center justify-between gap-3">
    <div class="flex min-w-0 flex-1 items-center gap-3">
      <slot name="prefix" />
      <div class="min-w-0 flex-1">
        <div v-if="status || provider" class="mb-1 flex flex-wrap items-center gap-2">
          <AdminChip v-if="provider" :label="provider" tone="strong" />
          <AdminChip v-if="status" :label="status" :variant="statusVariant" />
        </div>
        <h3
          class="m-0 leading-[1.2] font-bold text-[var(--admin-text-1)]"
          :class="size === 'small' ? 'mt-0.5 text-[15px]' : 'text-[18px]'"
        >
          {{ title }}
        </h3>
        <p
          class="mt-1 mr-0 mb-0 ml-0 overflow-hidden text-xs text-ellipsis whitespace-nowrap text-[var(--admin-text-2)] opacity-80"
          :title="subtitle"
        >
          {{ subtitle }}
        </p>
      </div>
    </div>
    <div v-if="$slots.extra">
      <slot name="extra" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    provider?: string
    status?: string
    statusVariant?: 'neutral' | 'current' | 'enabled' | 'empty'
    title: string
    subtitle: string
    size?: 'default' | 'small'
  }>(),
  {
    provider: '',
    status: '',
    statusVariant: 'neutral',
    size: 'default',
  },
)
</script>
