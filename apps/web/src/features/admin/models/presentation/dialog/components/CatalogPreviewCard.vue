<template>
  <div class="flex flex-col gap-4 lg:col-span-5">
    <!-- 实时卡片预览 -->
    <div
      class="flex flex-col gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted/30 p-4"
    >
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold text-admin-text-2">{{
          MODELS_MESSAGES.previewTitle
        }}</span>
        <span
          class="inline-flex items-center rounded bg-admin-accent/10 px-1.5 py-0.5 text-admin-micro font-semibold text-admin-accent"
        >
          Live Preview
        </span>
      </div>

      <!-- 模型卡片 (主体与列表页共用 AdminModelCardBody) -->
      <div
        class="flex flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-white shadow-xs"
      >
        <AdminModelCardBody :model="model" />
        <!-- 底栏 -->
        <div
          class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg/50 px-4 py-2 text-xs"
        >
          <span class="text-admin-caption text-admin-text-3">{{
            MODELS_MESSAGES.previewModelId(modelId)
          }}</span>
          <span class="text-admin-caption font-medium text-admin-accent">{{
            MODELS_MESSAGES.previewBadge
          }}</span>
        </div>
      </div>
    </div>

    <!-- 网关路由透视卡 (Cloudflare AI Gateway Route Inspector) -->
    <div
      class="flex flex-col gap-2.5 rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs"
    >
      <div class="flex items-center gap-2">
        <Globe class="size-4 text-admin-accent" />
        <span class="text-xs font-semibold text-admin-text-1">{{
          MODELS_MESSAGES.routeTitle
        }}</span>
      </div>

      <div class="flex flex-col gap-2 text-xs">
        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">{{ MODELS_MESSAGES.routeProviderLabel }}</span>
          <span class="font-mono font-medium text-admin-text-1">
            {{ routePreview.providerSlug }}
            <span
              v-if="routePreview.isFirstParty"
              class="ml-1 inline-flex rounded bg-emerald-100 px-1 text-admin-micro font-medium text-emerald-700"
            >
              {{ MODELS_MESSAGES.routeFirstParty }}
            </span>
            <span
              v-else
              class="ml-1 inline-flex rounded bg-slate-200 px-1 text-admin-micro font-medium text-slate-700"
            >
              {{ MODELS_MESSAGES.routeCustomNamespace }}
            </span>
          </span>
        </div>

        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">{{ MODELS_MESSAGES.routePathLabel }}</span>
          <span class="font-mono text-admin-text-1 text-admin-caption">
            {{ routePreview.targetPath }}
          </span>
        </div>

        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">{{ MODELS_MESSAGES.routeProtocolLabel }}</span>
          <span class="font-medium text-admin-text-1">
            {{ routePreview.protocol }}
          </span>
        </div>

        <div
          class="mt-1 rounded-lg border border-dashed border-admin-border-soft p-2.5 text-admin-caption text-admin-text-3"
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
import type { AdminModelItem } from '../../../domain/catalogData'
import { MODELS_MESSAGES } from '../../pages/messages'

export interface GatewayRoutePreviewState {
  providerSlug: string
  isFirstParty: boolean
  targetPath: string
  protocol: string
}

defineProps<{
  model: AdminModelItem
  modelId: string
  routePreview: GatewayRoutePreviewState
}>()
</script>
