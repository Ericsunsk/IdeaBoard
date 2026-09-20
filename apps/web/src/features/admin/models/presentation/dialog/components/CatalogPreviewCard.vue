<template>
  <div class="flex flex-col gap-4 lg:col-span-5">
    <!-- 实时卡片预览 -->
    <div class="flex flex-col gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted/30 p-4">
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold text-admin-text-2">卡片效果实时预览</span>
        <span class="inline-flex items-center rounded bg-admin-accent/10 px-1.5 py-0.5 text-[10px] font-semibold text-admin-accent">
          Live Preview
        </span>
      </div>

      <!-- 模型卡片 (主体与列表页共用 AdminModelCardBody) -->
      <div class="flex flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-white shadow-xs">
        <AdminModelCardBody :model="model" />
        <!-- 底栏 -->
        <div class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg/50 px-4 py-2 text-xs">
          <span class="text-[11px] text-admin-text-3">ID: {{ modelId || '未配置' }}</span>
          <span class="text-[11px] font-medium text-admin-accent">预览视图</span>
        </div>
      </div>
    </div>

    <!-- 网关路由透视卡 (Cloudflare AI Gateway Route Inspector) -->
    <div class="flex flex-col gap-2.5 rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
      <div class="flex items-center gap-2">
        <Globe class="size-4 text-admin-accent" />
        <span class="text-xs font-semibold text-admin-text-1">Cloudflare AI Gateway 路由透视</span>
      </div>

      <div class="flex flex-col gap-2 text-xs">
        <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
          <span class="text-admin-text-3">提供商空间:</span>
          <span class="font-mono font-medium text-admin-text-1">
            {{ routePreview.providerSlug }}
            <span
              v-if="routePreview.isFirstParty"
              class="ml-1 inline-flex rounded bg-emerald-100 px-1 text-[10px] font-medium text-emerald-700"
            >
              原生第一方
            </span>
            <span
              v-else
              class="ml-1 inline-flex rounded bg-slate-200 px-1 text-[10px] font-medium text-slate-700"
            >
              自定义命名空间
            </span>
          </span>
        </div>

        <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
          <span class="text-admin-text-3">预期端点路径:</span>
          <span class="font-mono text-admin-text-1 text-[11px]">
            {{ routePreview.targetPath }}
          </span>
        </div>

        <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
          <span class="text-admin-text-3">调用调度协议:</span>
          <span class="font-medium text-admin-text-1">
            {{ routePreview.protocol }}
          </span>
        </div>

        <div class="mt-1 rounded-lg border border-dashed border-admin-border-soft p-2.5 text-[11px] text-admin-text-3">
          💡 系统后端采用统一 Cloudflare AI Gateway 驱动，无需配置第三方独立 SDK 或外部密钥池。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Globe } from '@iconoir/vue'
import AdminModelCardBody from '../../controls/AdminModelCardBody.vue'
import type { AdminModelItem } from '../../../domain/catalogData'

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
