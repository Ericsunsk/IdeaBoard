<template>
  <div :class="['relative', fluid ? 'min-w-[220px] max-w-md flex-1' : 'w-full sm:w-[260px]']">
    <Search
      class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground"
    />
    <AdminInput
      :model-value="modelValue"
      :aria-label="inputLabel"
      :placeholder="placeholder"
      class="pl-9 pr-8"
      @update:model-value="handleValueUpdate"
      @enter="$emit('enter')"
    />
    <AdminButton
      v-if="modelValue"
      variant="ghost"
      size="icon"
      class="absolute top-1/2 right-1 size-7 -translate-y-1/2 rounded-md text-muted-foreground hover:text-foreground"
      aria-label="清空搜索"
      @click="handleClear"
    >
      <Xmark class="size-3.5" />
    </AdminButton>
  </div>
</template>

<script setup lang="ts">
import { Search, Xmark } from '@iconoir/vue'
import AdminButton from './AdminButton.vue'
import AdminInput from './AdminInput.vue'

withDefaults(
  defineProps<{
    modelValue: string
    inputLabel: string
    placeholder?: string
    fluid?: boolean
  }>(),
  {
    placeholder: 'Search',
    fluid: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  clear: []
  enter: []
}>()

function handleValueUpdate(value: string | number | null | undefined) {
  emit('update:modelValue', String(value ?? ''))
}

function handleClear() {
  emit('update:modelValue', '')
  emit('clear')
}
</script>
