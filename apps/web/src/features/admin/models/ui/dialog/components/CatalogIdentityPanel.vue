<template>
  <div class="rounded-panel border border-border bg-card p-4 shadow-raised">
    <div class="mb-3 flex items-center justify-between border-b border-border pb-2.5">
      <h4 class="text-xs font-semibold text-foreground">{{ MODELS_MESSAGES.identityTitle }}</h4>
      <span class="text-admin-caption text-muted-foreground">{{
        MODELS_MESSAGES.identitySubtitle
      }}</span>
    </div>

    <div class="flex flex-col gap-3.5">
      <!-- 模型标识 (ID) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-admin-caption font-semibold text-foreground" for="model-id-input">
            {{ MODELS_MESSAGES.identityIdLabel }} <span class="text-destructive">*</span>
          </label>
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.identityIdHint
          }}</span>
        </div>
        <AdminInput
          id="model-id-input"
          :model-value="modelId"
          :placeholder="MODELS_MESSAGES.identityIdPlaceholder"
          :disabled="isEditing"
          @update:model-value="emit('update:modelId', String($event))"
        />
      </div>

      <!-- 模型显示名称 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-foreground" for="model-name-input">
          {{ MODELS_MESSAGES.identityNameLabel }} <span class="text-destructive">*</span>
        </label>
        <AdminInput
          id="model-name-input"
          :model-value="modelName"
          :placeholder="MODELS_MESSAGES.identityNamePlaceholder"
          @update:model-value="emit('update:modelName', String($event))"
        />
      </div>

      <!-- 提供商 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-foreground" for="model-provider-input">
          {{ MODELS_MESSAGES.identityProviderLabel }} <span class="text-destructive">*</span>
        </label>
        <AdminInput
          id="model-provider-input"
          :model-value="modelProvider"
          :placeholder="MODELS_MESSAGES.identityProviderPlaceholder"
          @update:model-value="emit('update:modelProvider', String($event))"
        />
        <!-- 预设提供商快速选择（Registry 驱动，存 slug 真值） -->
        <div v-if="providerOptions.length > 0" class="flex flex-wrap gap-1 pt-0.5">
          <button
            v-for="p in providerOptions"
            :key="p.slug"
            type="button"
            class="rounded-control px-1.5 py-0.5 text-admin-micro font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer"
            :class="
              modelProvider === p.slug
                ? 'bg-primary text-primary-foreground font-semibold'
                : 'bg-muted text-muted-foreground hover:bg-muted/80'
            "
            @click="selectProvider(p)"
          >
            {{ p.display_name }}
          </button>
        </div>
      </div>

      <!-- 功能描述 -->
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-foreground" for="model-desc-input">
          {{ MODELS_MESSAGES.identityDescriptionLabel }}
        </label>
        <AdminInput
          id="model-desc-input"
          :model-value="modelDescription"
          :placeholder="MODELS_MESSAGES.identityDescriptionPlaceholder"
          @update:model-value="emit('update:modelDescription', String($event))"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { AdminInput } from '../../../../shared'
import type { GatewayProviderOption } from '../../../resource'
import { MODELS_MESSAGES } from '../../pages/messages'

defineProps<{
  modelId: string
  modelName: string
  modelProvider: string
  modelDescription: string
  isEditing: boolean
  providerOptions: GatewayProviderOption[]
}>()

const emit = defineEmits<{
  'update:modelId': [value: string]
  'update:modelName': [value: string]
  'update:modelProvider': [value: string]
  'update:modelDescription': [value: string]
}>()

// provider 快捷选择存 registry slug 真值（展示 display_name）。
function selectProvider(p: GatewayProviderOption) {
  emit('update:modelProvider', p.slug)
}
</script>
