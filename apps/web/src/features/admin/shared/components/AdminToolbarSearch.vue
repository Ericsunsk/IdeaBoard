<template>
  <IconField class="relative w-full sm:w-[260px]">
    <InputIcon>
      <Search class="size-4 text-admin-text-3" />
    </InputIcon>
    <InputText
      :model-value="modelValue"
      :placeholder="placeholder"
      fluid
      :class="adminToolbarSearchInputClass"
      @update:model-value="handleValueUpdate"
      @keydown.enter="$emit('enter')"
    />
    <Button
      v-if="modelValue"
      text
      rounded
      :class="[adminTextButtonClass, 'absolute top-1/2 right-1 !h-8 !w-8 -translate-y-1/2']"
      aria-label="清空搜索"
      @click="handleClear"
    >
      <template #icon>
        <Xmark />
      </template>
    </Button>
  </IconField>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import { Search, Xmark } from '@iconoir/vue'
import { adminTextButtonClass, adminToolbarSearchInputClass } from '../lib/adminUiClasses'

withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
  }>(),
  {
    placeholder: 'Search',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  clear: []
  enter: []
}>()

function handleValueUpdate(value: string | undefined) {
  emit('update:modelValue', value ?? '')
}

function handleClear() {
  emit('update:modelValue', '')
  emit('clear')
}
</script>
