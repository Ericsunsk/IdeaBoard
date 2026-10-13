<template>
  <div class="relative w-full">
    <Input
      :id="props.id"
      v-model="modelValue"
      :type="showPassword ? 'text' : 'password'"
      :name="props.name"
      :placeholder="props.placeholder"
      :disabled="props.disabled"
      :required="props.required"
      :autocomplete="props.autocomplete"
      :invalid="props.invalid"
      :class="cn('pr-10', props.inputClass, props.class)"
      :aria-label="props.ariaLabel"
      :aria-invalid="props.ariaInvalid"
      :aria-describedby="props.ariaDescribedby"
    />
    <button
      type="button"
      :aria-label="toggleAriaLabel"
      :title="toggleAriaLabel"
      class="control-state absolute right-0 top-0 flex h-full cursor-pointer items-center justify-center px-3 text-muted-foreground hover:text-foreground"
      @click="showPassword = !showPassword"
    >
      <EyeClosed v-if="showPassword" class="h-4 w-4" />
      <Eye v-else class="h-4 w-4" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, type HTMLAttributes } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeClosed } from '@iconoir/vue'
import { cn } from '@/lib/utils'
import Input from './Input.vue'

const props = withDefaults(
  defineProps<{
    id?: string | undefined
    name?: string | undefined
    placeholder?: string | undefined
    disabled?: boolean
    required?: boolean
    autocomplete?: string | undefined
    class?: HTMLAttributes['class'] | undefined
    inputClass?: HTMLAttributes['class'] | undefined
    ariaLabel?: string | undefined
    ariaInvalid?: boolean | 'true' | 'false' | undefined
    ariaDescribedby?: string | undefined
    invalid?: boolean
  }>(),
  {
    id: undefined,
    name: undefined,
    placeholder: undefined,
    disabled: false,
    required: false,
    autocomplete: undefined,
    class: undefined,
    inputClass: undefined,
    ariaLabel: undefined,
    ariaInvalid: undefined,
    ariaDescribedby: undefined,
    invalid: false,
  },
)

const modelValue = defineModel<string | number | undefined>()

const { t } = useI18n()
const toggleAriaLabel = computed(() =>
  showPassword.value ? t('common.a11y.hidePassword') : t('common.a11y.showPassword'),
)

const showPassword = ref(false)
</script>
