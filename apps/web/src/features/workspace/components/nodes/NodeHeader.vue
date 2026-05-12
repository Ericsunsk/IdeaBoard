<template>
  <div
    class="workflow-node-header"
    :class="rounded ? 'rounded-t-[12px]' : ''"
    @mousedown="emit('node-mousedown', $event)"
  >
    <div class="flex items-center">
      <span v-if="$slots.icon" class="mr-1.5 inline-flex opacity-80 [&_svg]:block">
        <slot name="icon" />
      </span>
      {{ title }}
    </div>
    <button
      class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-md border-none bg-transparent text-[#94a3b8] transition-all hover:bg-[rgba(239,68,68,0.08)] hover:text-[#ef4444]"
      title="删除"
      @mousedown.stop
      @click.stop="emit('delete')"
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
      >
        <line x1="18" y1="6" x2="6" y2="18" />
        <line x1="6" y1="6" x2="18" y2="18" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  rounded?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
}>()
</script>

<style scoped src="./node-shared.css"></style>
