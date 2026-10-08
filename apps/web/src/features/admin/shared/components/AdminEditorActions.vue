<template>
  <!-- shadcn DialogFooter 同款语义：次要 outline 在前、主要压轴在右；移动端 col-reverse 让保存置顶 -->
  <div
    class="flex w-full flex-wrap items-center gap-2"
    :class="$slots.leading ? 'justify-between' : 'justify-end'"
  >
    <slot name="leading" />
    <div class="flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center">
      <ActionButton
        v-if="resetLabel"
        variant="outline"
        :disabled="resetDisabled"
        @click="emit('reset')"
      >
        {{ resetLabel }}
      </ActionButton>
      <ActionButton :disabled="saveDisabled" :loading="saving" @click="emit('save')">
        {{ saveLabel }}
      </ActionButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import ActionButton from '@/shared/components/ActionButton.vue'

withDefaults(
  defineProps<{
    saveLabel?: string | undefined
    resetLabel?: string | undefined
    saving?: boolean | undefined
    saveDisabled?: boolean | undefined
    resetDisabled?: boolean | undefined
  }>(),
  {
    // 绝大多数 editor 卡都是 [重置][保存]，默认值让调用方无需手写；
    // 特殊文案仍可通过 props 覆盖。
    saveLabel: '保存',
    resetLabel: '重置',
    saving: false,
    saveDisabled: false,
    resetDisabled: false,
  },
)

const emit = defineEmits<{
  reset: []
  save: []
}>()
</script>
