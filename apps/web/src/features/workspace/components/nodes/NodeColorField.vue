<template>
  <div class="relative flex items-center gap-2">
    <button
      type="button"
      class="inline-flex h-[30px] w-[30px] shrink-0 cursor-pointer appearance-none items-center justify-center overflow-hidden rounded-full border border-[#d6dcec] bg-white p-0 transition-colors hover:border-[#b6a6f0]"
      :style="{ background: value }"
      :aria-label="swatchAriaLabel"
      @click.stop="emit('swatch-click')"
    >
      <NodeColorSwatch :color="value" :size="30" borderless />
    </button>
    <input
      :value="value"
      class="h-[30px] min-w-0 flex-1 rounded-[8px] border border-[#d6dcec] bg-white px-2 font-mono text-xs text-[#425067] uppercase outline-none focus:border-[#9278e6]"
      type="text"
      maxlength="7"
      :placeholder="placeholder"
      @input="emit('update:value', ($event.target as HTMLInputElement).value)"
      @change="emit('change:value', ($event.target as HTMLInputElement).value)"
    />
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    value: string
    placeholder?: string
    swatchAriaLabel?: string
  }>(),
  {
    placeholder: '#000000',
    swatchAriaLabel: '选择颜色',
  },
)

const emit = defineEmits<{
  (e: 'swatch-click'): void
  (e: 'update:value', value: string): void
  (e: 'change:value', value: string): void
}>()
</script>
