<template>
  <div class="rounded-xl border border-border bg-muted/20 p-4">
    <button
      type="button"
      class="flex w-full items-center justify-between text-xs font-semibold text-muted-foreground hover:text-foreground cursor-pointer"
      @click="isOpen = !isOpen"
    >
      <div class="flex items-center gap-2">
        <Settings class="size-4 text-muted-foreground" />
        <span>{{ MODELS_MESSAGES.gatewayAdvancedTitle }}</span>
      </div>
      <NavArrowDown class="size-4 transition-transform" :class="isOpen ? 'rotate-180' : ''" />
    </button>

    <div v-if="isOpen" class="mt-3 flex flex-col gap-3.5 border-t border-border pt-3">
      <!-- 自定义网关路径 (Path Override) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="model-custom-path-input"
          >
            {{ MODELS_MESSAGES.gatewayPathLabel }}
          </label>
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.gatewayPathHint
          }}</span>
        </div>
        <AdminInput
          id="model-custom-path-input"
          :model-value="customPath"
          :placeholder="MODELS_MESSAGES.gatewayPathPlaceholder"
          @update:model-value="emit('update:customPath', String($event))"
        />
        <div class="flex flex-wrap gap-1">
          <button
            v-for="pathOpt in commonGatewayPaths"
            :key="pathOpt"
            type="button"
            class="rounded bg-card border border-border px-1.5 py-0.5 text-admin-micro text-muted-foreground hover:text-primary cursor-pointer"
            @click="emit('update:customPath', pathOpt)"
          >
            {{ pathOpt }}
          </button>
        </div>
      </div>

      <!-- 额外自定义参数 (JSON) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="model-custom-params-input"
          >
            {{ MODELS_MESSAGES.gatewayParamsLabel }}
          </label>
          <button
            v-if="customParams"
            type="button"
            class="text-admin-caption text-primary hover:underline cursor-pointer"
            @click="formatCustomParamsJSON"
          >
            {{ MODELS_MESSAGES.gatewayFormatJson }}
          </button>
        </div>
        <AdminInput
          id="model-custom-params-input"
          :model-value="customParams"
          :placeholder="MODELS_MESSAGES.gatewayParamsPlaceholder"
          @update:model-value="emit('update:customParams', String($event))"
        />
        <span class="text-admin-caption text-muted-foreground">
          {{ MODELS_MESSAGES.gatewayParamsHint }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { NavArrowDown, Settings } from '@iconoir/vue'
import AdminInput from '../../../../shared/components/AdminInput.vue'
import { MODELS_MESSAGES } from '../../pages/messages'

const props = withDefaults(
  defineProps<{
    customPath?: string | undefined
    customParams?: string | undefined
    initiallyOpen?: boolean
  }>(),
  {
    customPath: '',
    customParams: '',
    initiallyOpen: false,
  },
)

const emit = defineEmits<{
  'update:customPath': [value: string]
  'update:customParams': [value: string]
}>()

const isOpen = ref(Boolean(props.initiallyOpen))

watch(
  () => props.initiallyOpen,
  (val) => {
    if (val !== undefined) {
      isOpen.value = val
    }
  },
)

const commonGatewayPaths = [
  'api/v1/chat/completions',
  'v1/chat/completions',
  'v1/images/generations',
  'v1/images/cutout',
  'v1/images/upscale',
]

function formatCustomParamsJSON() {
  try {
    const parsed = JSON.parse(props.customParams || '{}')
    emit('update:customParams', JSON.stringify(parsed, null, 2))
  } catch {
    // ignore format errors
  }
}
</script>
