<template>
  <div class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/20 p-4">
    <button
      type="button"
      class="flex w-full items-center justify-between text-xs font-semibold text-admin-text-2 hover:text-admin-text-1 cursor-pointer"
      @click="isOpen = !isOpen"
    >
      <div class="flex items-center gap-2">
        <Settings class="size-4 text-admin-text-3" />
        <span>高级网关适配与参数微调 (可选)</span>
      </div>
      <NavArrowDown class="size-4 transition-transform" :class="isOpen ? 'rotate-180' : ''" />
    </button>

    <div v-if="isOpen" class="mt-3 flex flex-col gap-3.5 border-t border-admin-border-soft pt-3">
      <!-- 自定义网关路径 (Path Override) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-admin-text-1"
            for="model-custom-path-input"
          >
            自定义网关端点路径 (Path Override)
          </label>
          <span class="text-[11px] text-admin-text-3">留空则由系统智能推导</span>
        </div>
        <AdminInput
          id="model-custom-path-input"
          :model-value="customPath"
          placeholder="例如: api/v1/chat/completions 或 fal-ai/birefnet"
          @update:model-value="emit('update:customPath', String($event))"
        />
        <div class="flex flex-wrap gap-1">
          <button
            v-for="pathOpt in commonGatewayPaths"
            :key="pathOpt"
            type="button"
            class="rounded bg-white border border-admin-border-soft px-1.5 py-0.5 text-[10px] text-admin-text-2 hover:text-admin-accent cursor-pointer"
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
            class="text-admin-caption font-semibold text-admin-text-1"
            for="model-custom-params-input"
          >
            额外自定义请求参数 (Custom JSON)
          </label>
          <button
            v-if="customParams"
            type="button"
            class="text-[11px] text-admin-accent hover:underline cursor-pointer"
            @click="formatCustomParamsJSON"
          >
            格式化 JSON
          </button>
        </div>
        <AdminInput
          id="model-custom-params-input"
          :model-value="customParams"
          placeholder='例如: {"guidance_scale": 7.5, "output_format": "png"}'
          @update:model-value="emit('update:customParams', String($event))"
        />
        <span class="text-[11px] text-admin-text-3">
          必须为合法 JSON 键值对对象。调用网关时将自动与基础 Payload 进行深度合并注入。
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { NavArrowDown, Settings } from '@iconoir/vue'
import AdminInput from '../../../../shared/components/AdminInput.vue'

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
