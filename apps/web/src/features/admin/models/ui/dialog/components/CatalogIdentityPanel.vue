<template>
  <div class="rounded-xl border border-border bg-card p-4 shadow-xs">
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
        <!-- 快捷填充供应商前缀（Registry 驱动） -->
        <div
          v-if="!isEditing && idPrefixOptions.length > 0"
          class="flex flex-wrap items-center gap-1 pt-0.5"
        >
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.identityPrefixLabel
          }}</span>
          <button
            v-for="prefix in idPrefixOptions"
            :key="prefix"
            type="button"
            class="rounded-md border border-border bg-muted/50 px-1.5 py-0.5 text-admin-caption font-medium text-muted-foreground transition-colors hover:border-primary-line-strong hover:bg-primary-faint-soft hover:text-primary cursor-pointer"
            @click="applyPrefix(prefix)"
          >
            {{ prefix }}
          </button>
        </div>
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
        <!-- 预设提供商快速选择（Registry 驱动，无数据时仅手输） -->
        <div v-if="providerOptions.length > 0" class="flex flex-wrap gap-1 pt-0.5">
          <button
            v-for="p in providerOptions"
            :key="p.slug"
            type="button"
            class="rounded px-1.5 py-0.5 text-admin-micro font-medium transition-colors cursor-pointer"
            :class="
              modelProvider === p.display_name
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
