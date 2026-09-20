<template>
  <PopoverRoot v-model:open="isOpen">
    <PopoverTrigger as-child>
      <button
        type="button"
        class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        :class="active ? 'border-admin-accent text-admin-accent bg-admin-accent/5' : ''"
        :aria-label="label"
        :aria-expanded="isOpen"
        aria-haspopup="listbox"
      >
        <span>{{ selectedLabel }}</span>
        <NavArrowDown
          class="size-3.5 text-admin-text-3 transition-transform"
          :class="isOpen ? 'rotate-180' : ''"
        />
      </button>
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        align="start"
        :side-offset="4"
        class="z-50 rounded-xl border border-admin-border bg-admin-card-strong p-1 text-admin-text-1 outline-none"
      >
        <div class="flex min-w-[130px] flex-col gap-0.5" role="listbox" :aria-label="label">
          <button
            v-for="option in options"
            :key="option.value"
            type="button"
            role="option"
            :aria-selected="modelValue === option.value"
            :class="[
              'flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer',
              modelValue === option.value
                ? 'bg-admin-accent/10 text-admin-accent font-semibold'
                : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
            ]"
            @click="select(option.value)"
          >
            <span>{{ option.label }}</span>
            <Check v-if="modelValue === option.value" class="size-3.5 text-admin-accent shrink-0" />
          </button>
        </div>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, NavArrowDown } from '@iconoir/vue'
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'

export type AdminOptionFilterOption = {
  label: string
  value: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: readonly AdminOptionFilterOption[]
    allValue?: string
    label: string
  }>(),
  { allValue: '' },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const isOpen = ref(false)
const active = computed(() => props.modelValue !== props.allValue)
const selectedLabel = computed(
  () => props.options.find((option) => option.value === props.modelValue)?.label ?? props.options[0]?.label ?? '',
)

function select(value: string) {
  emit('update:modelValue', value)
  isOpen.value = false
}
</script>
