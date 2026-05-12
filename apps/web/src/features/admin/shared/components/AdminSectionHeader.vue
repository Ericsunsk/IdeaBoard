<template>
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h3 class="m-0 text-base font-semibold text-[var(--admin-text-1)]">{{ title }}</h3>
      <p
        v-if="description || $slots.description"
        class="mt-1.5 mb-0 text-[13px] text-[var(--admin-text-2)]"
      >
        <slot name="description">{{ description }}</slot>
      </p>
    </div>
    <slot />
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
    variant?: 'panel' | 'section'
  }>(),
  {
    description: '',
    variant: 'panel',
  },
)
</script>
