<template>
  <div class="node-connection-stack">
    <div class="node-connection-status">
      <span
        v-for="item in visibleItems"
        :key="item.key"
        class="node-connection-chip"
        :class="[
          `is-${item.kind}`,
          {
            'is-connected': !!item.connected,
            'is-pending': !!item.pending,
            'is-error': !!item.error,
            'is-required': !!item.required,
          },
        ]"
      >
        {{ item.label }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
type ChipKind = 'image' | 'mask' | 'prompt'

type ConnectionChipItem = {
  key: string
  label: string
  kind: ChipKind
  connected?: boolean
  pending?: boolean
  error?: boolean
  required?: boolean
  visible?: boolean
}

const props = defineProps<{
  items: ConnectionChipItem[]
}>()

const visibleItems = computed(() => props.items.filter((item) => item.visible !== false))
</script>

<style scoped src="./node-connection-chips.css"></style>
