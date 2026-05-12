<template>
  <div class="node-connection-stack flex flex-col gap-1">
    <div class="node-connection-status flex flex-wrap gap-1.5">
      <span
        v-for="item in visibleItems"
        :key="item.key"
        class="node-connection-chip inline-flex h-[20px] items-center gap-[5px] rounded-full border border-slate-200 bg-white px-[7px] text-[10px] leading-none font-bold tracking-[0.02em] text-slate-500 transition-[color,border-color,background-color,box-shadow] duration-[160ms] ease-in-out"
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
