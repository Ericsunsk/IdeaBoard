<template>
  <div class="flex min-w-0 flex-col gap-1.5">
    <div class="flex min-w-0 items-center gap-2">
      <AdminModelSelect
        :id="`model-select-${id}`"
        :model-value="modelValue"
        :models="availableModels"
        :disabled="modelPending"
        :allow-custom="false"
        :aria-label="`模型（${title}）`"
        @select="emit('select', $event)"
      />
    </div>
    <span v-if="modelError" class="text-admin-caption text-admin-danger" role="alert">
      {{ modelError }}
    </span>
  </div>
</template>

<script setup lang="ts">
import AdminModelSelect from '../../models/presentation/controls/AdminModelSelect.vue'
import type { AdminModelItem } from '../../models/domain/catalogData'

// 定价页模型映射行：pricing 内唯一渲染 AdminModelSelect 的地方
//（模型域 sanctioned 出口 allowCustom=false）。状态全由 props 进入，
// 读写经 usePricingModelAdapter，Card 不再直连模型域。
withDefaults(
  defineProps<{
    id: string
    title: string
    modelValue?: string | undefined
    modelPending?: boolean | undefined
    modelError?: string | undefined
    availableModels: AdminModelItem[]
  }>(),
  {
    modelValue: undefined,
    modelPending: undefined,
    modelError: undefined,
  },
)

const emit = defineEmits<{
  select: [modelId: string]
}>()
</script>
