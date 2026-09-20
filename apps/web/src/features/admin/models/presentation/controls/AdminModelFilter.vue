<template>
  <AdminPopover v-model:open="isOpen">
    <AdminPopoverTrigger as-child>
      <button
        type="button"
        class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        :class="modelValue ? 'border-admin-accent text-admin-accent bg-admin-accent/5' : ''"
        :aria-label="ariaLabel ?? `筛选${label}`"
      >
        <span>{{ modelValue || label }}</span>
        <NavArrowDown
          class="size-3.5 text-admin-text-3 transition-transform"
          :class="isOpen ? 'rotate-180' : ''"
        />
      </button>
    </AdminPopoverTrigger>
    <AdminPopoverContent align="start" :side-offset="4">
      <div class="flex flex-col gap-0.5 min-w-[140px]" role="listbox">
        <button
          type="button"
          role="option"
          :aria-selected="modelValue === ''"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer',
            modelValue === ''
              ? 'bg-admin-accent/10 text-admin-accent font-semibold'
              : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
          ]"
          @click="select('')"
        >
          <span>{{ allLabel }}</span>
          <Check v-if="modelValue === ''" class="size-3.5 text-admin-accent shrink-0" />
        </button>
        <button
          v-for="item in options"
          :key="item"
          type="button"
          role="option"
          :aria-selected="modelValue === item"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer',
            modelValue === item
              ? 'bg-admin-accent/10 text-admin-accent font-semibold'
              : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
          ]"
          @click="select(item)"
        >
          <span>{{ item }}</span>
          <Check v-if="modelValue === item" class="size-3.5 text-admin-accent shrink-0" />
        </button>
      </div>
    </AdminPopoverContent>
  </AdminPopover>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Check, NavArrowDown } from '@iconoir/vue'
import AdminPopover from '../../../shared/components/AdminPopover.vue'
import AdminPopoverContent from '../../../shared/components/AdminPopoverContent.vue'
import AdminPopoverTrigger from '../../../shared/components/AdminPopoverTrigger.vue'

defineProps<{
  label: string
  allLabel: string
  options: readonly string[]
  ariaLabel?: string | undefined
}>()

const modelValue = defineModel<string>({ required: true })
const isOpen = ref(false)

function select(value: string) {
  modelValue.value = value
  isOpen.value = false
}
</script>
