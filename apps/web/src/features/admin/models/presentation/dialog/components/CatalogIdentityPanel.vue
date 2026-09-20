<template>
  <div class="rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
    <div class="mb-3 flex items-center justify-between border-b border-admin-border-soft pb-2.5">
      <h4 class="text-xs font-semibold text-admin-text-1">模型身份与接入定义</h4>
      <span class="text-[11px] text-admin-text-3">Cloudflare AI Gateway 路由基础</span>
    </div>

    <div class="flex flex-col gap-3.5">
      <!-- 模型标识 (ID) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-admin-caption font-semibold text-admin-text-1" for="model-id-input">
            模型标识 <span class="text-admin-danger">*</span>
          </label>
          <span class="text-[11px] text-admin-text-3">建议 provider/model 格式</span>
        </div>
        <AdminInput
          id="model-id-input"
          :model-value="modelId"
          placeholder="例如: fal/birefnet 或 openai/gpt-image-1"
          :disabled="isEditing"
          @update:model-value="emit('update:modelId', String($event))"
        />
        <!-- 快捷填充供应商前缀（Registry 驱动） -->
        <div v-if="!isEditing && idPrefixOptions.length > 0" class="flex flex-wrap items-center gap-1 pt-0.5">
          <span class="text-[11px] text-admin-text-3">常用前缀:</span>
          <button
            v-for="prefix in idPrefixOptions"
            :key="prefix"
            type="button"
            class="rounded-md border border-admin-border-soft bg-admin-surface-muted/50 px-1.5 py-0.5 text-[11px] font-medium text-admin-text-2 transition-colors hover:border-admin-accent/40 hover:bg-admin-accent/5 hover:text-admin-accent cursor-pointer"
            @click="applyPrefix(prefix)"
          >
            {{ prefix }}
          </button>
        </div>
      </div>

      <!-- 模型显示名称 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-admin-text-1" for="model-name-input">
          显示名称 <span class="text-admin-danger">*</span>
        </label>
        <AdminInput
          id="model-name-input"
          :model-value="modelName"
          placeholder="例如: FLUX.1 [dev] 旗舰版"
          @update:model-value="emit('update:modelName', String($event))"
        />
      </div>

      <!-- 提供商 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-admin-text-1" for="model-provider-input">
          提供商 (Provider) <span class="text-admin-danger">*</span>
        </label>
        <AdminInput
          id="model-provider-input"
          :model-value="modelProvider"
          placeholder="例如: Fal AI, OpenAI"
          @update:model-value="emit('update:modelProvider', String($event))"
        />
        <!-- 预设提供商快速选择（Registry 驱动，无数据时仅手输） -->
        <div v-if="providerOptions.length > 0" class="flex flex-wrap gap-1 pt-0.5">
          <button
            v-for="p in providerOptions"
            :key="p.slug"
            type="button"
            class="rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors cursor-pointer"
            :class="
              modelProvider === p.display_name
                ? 'bg-admin-accent text-white font-semibold'
                : 'bg-admin-surface-muted text-admin-text-2 hover:bg-admin-surface-muted/80'
            "
            @click="selectProvider(p)"
          >
            {{ p.display_name }}
          </button>
        </div>
      </div>

      <!-- 功能描述 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-admin-text-1" for="model-desc-input">
          功能说明与业务描述
        </label>
        <AdminInput
          id="model-desc-input"
          :model-value="modelDescription"
          placeholder="例如: 针对电商白底图与高质感工业设计柔光渲染深度优化..."
          @update:model-value="emit('update:modelDescription', String($event))"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import AdminInput from '../../../../shared/components/AdminInput.vue'
import type { GatewayProviderOption } from '../../../infrastructure/adminGatewayResource'

const props = defineProps<{
  modelId: string
  modelName: string
  modelProvider: string
  modelDescription: string
  isEditing: boolean
  idPrefixOptions: string[]
  providerOptions: GatewayProviderOption[]
}>()

const emit = defineEmits<{
  'update:modelId': [value: string]
  'update:modelName': [value: string]
  'update:modelProvider': [value: string]
  'update:modelDescription': [value: string]
}>()

function applyPrefix(prefix: string) {
  if (!props.modelId) {
    emit('update:modelId', prefix)
  } else if (!props.modelId.includes('/')) {
    emit('update:modelId', prefix + props.modelId)
  }
}

function selectProvider(p: GatewayProviderOption) {
  emit('update:modelProvider', p.display_name)
  if (p.id_prefix && !props.modelId) {
    emit('update:modelId', p.id_prefix)
  }
}
</script>
