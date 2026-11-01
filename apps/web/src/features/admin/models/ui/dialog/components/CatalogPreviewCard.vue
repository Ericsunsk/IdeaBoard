<template>
  <div class="flex flex-col gap-4 lg:col-span-5">
    <!-- 实时卡片预览 -->
    <div class="flex flex-col gap-2 rounded-panel border border-border bg-muted/30 p-4">
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold text-muted-foreground">{{
          MODELS_MESSAGES.previewTitle
        }}</span>
        <span
          class="inline-flex items-center rounded-control bg-primary-faint px-1.5 py-0.5 text-admin-micro font-semibold text-primary"
        >
          Live Preview
        </span>
      </div>

      <!-- 模型卡片 (主体与列表页共用 AdminModelCardBody) -->
      <div
        class="flex flex-col overflow-hidden rounded-panel border border-border bg-card shadow-raised"
      >
        <AdminModelCardBody :model="model" />
        <!-- 底栏 -->
        <div
          class="flex items-center justify-between border-t border-border bg-background/50 px-4 py-2 text-xs"
        >
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.previewModelId(modelId)
          }}</span>
          <span class="text-admin-caption font-medium text-primary">{{
            MODELS_MESSAGES.previewBadge
          }}</span>
        </div>
      </div>
    </div>

    <!-- Worker 干跑透视卡 (Edge Worker Dry-run Inspector) -->
    <div class="flex flex-col gap-2.5 rounded-panel border border-border bg-card p-4 shadow-raised">
      <div class="flex items-center gap-2">
        <Globe class="size-4 text-primary" />
        <span class="text-xs font-semibold text-foreground">{{ MODELS_MESSAGES.routeTitle }}</span>
      </div>

      <div class="flex flex-col gap-2 text-xs">
        <div class="flex items-center justify-between rounded-panel bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.routeProviderLabel }}</span>
          <span class="font-mono font-medium text-foreground">
            {{ routePreview.provider || MODELS_MESSAGES.editRouteUnspecified }}
            <span class="ml-1 font-mono text-admin-caption text-muted-foreground">
              {{ routePreview.model }}
            </span>
          </span>
        </div>

        <div class="flex items-center justify-between rounded-panel bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.routeValidityLabel }}</span>
          <span
            class="inline-flex rounded-control px-1 text-admin-micro font-medium"
            :class="
              routePreview.isValid
                ? 'bg-success-soft text-success'
                : 'bg-destructive-soft text-destructive'
            "
          >
            {{ routePreview.isValid ? MODELS_MESSAGES.routeValid : MODELS_MESSAGES.routeInvalid }}
          </span>
        </div>

        <div class="flex items-center justify-between rounded-panel bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.routeDryRunLabel }}</span>
          <span class="font-medium text-foreground">
            {{ routePreview.message }}
          </span>
        </div>

        <div
          class="mt-1 rounded-panel border border-dashed border-border p-2.5 text-admin-caption text-muted-foreground"
        >
          {{ MODELS_MESSAGES.routeNote }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Globe } from '@iconoir/vue'
import AdminModelCardBody from '../../controls/AdminModelCardBody.vue'
import type { AdminModelItem } from '../../../core/catalogData'
import { MODELS_MESSAGES } from '../../pages/messages'

export interface WorkerPreviewState {
  provider: string
  model: string
  isValid: boolean
  message: string
}

defineProps<{
  model: AdminModelItem
  modelId: string
  routePreview: WorkerPreviewState
}>()
</script>
