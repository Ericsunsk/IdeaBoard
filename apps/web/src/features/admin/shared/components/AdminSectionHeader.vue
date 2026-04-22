<template>
  <div :class="headerClass">
    <div>
      <h3 class="panel-title">{{ title }}</h3>
      <p v-if="description || $slots.description" class="panel-desc">
        <slot name="description">{{ description }}</slot>
      </p>
    </div>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  variant?: 'panel' | 'section'
}>(), {
  description: '',
  variant: 'panel',
})

const headerClass = computed(() => props.variant === 'section' ? 'section-header' : 'panel-header')
</script>

<style scoped>
.panel-header,
.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.panel-title {
  margin: 0;
  font-size: 16px;
  color: var(--admin-text-1);
}

.panel-desc {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--admin-text-2);
}
</style>
