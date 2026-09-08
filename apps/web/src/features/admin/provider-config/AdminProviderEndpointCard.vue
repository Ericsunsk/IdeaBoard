<template>
  <Card :class="[adminMasonryCardClass, 'min-w-0']" :pt="adminCardPt">
    <template #title>
      <div class="flex items-start justify-between gap-3">
        <div class="flex min-w-0 items-center gap-3">
          <button
            type="button"
            :class="[adminLogoButtonClass, 'size-10 p-2']"
            :data-control-state="activeLogoTargetId === card.providerEndpoint.id ? 'selected' : 'idle'"
            :aria-expanded="activeLogoTargetId === card.providerEndpoint.id ? 'true' : 'false'"
            title="点击切换端点 Logo"
            aria-label="切换端点 Logo"
            @click="emit('open-logo-picker', $event, card)"
          >
            <img
              v-if="
                getProviderEndpointLogo(
                  {
                    name: card.providerEndpoint.name,
                    baseUrl: card.providerEndpoint.baseUrl,
                    apiType: card.providerEndpoint.apiType,
                  },
                  card.providerEndpoint.id,
                )
              "
              :src="
                getProviderEndpointLogo(
                  {
                    name: card.providerEndpoint.name,
                    baseUrl: card.providerEndpoint.baseUrl,
                    apiType: card.providerEndpoint.apiType,
                  },
                  card.providerEndpoint.id,
                )
              "
              :alt="card.providerEndpoint.name || 'Provider'"
              class="size-full object-contain"
            />
            <span v-else class="text-xs font-bold text-admin-accent uppercase select-none">
              {{ (card.providerEndpoint.name || 'EP').substring(0, 2).toUpperCase() }}
            </span>
          </button>
          <div class="min-w-0">
            <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
              {{ card.providerEndpoint.name }}
            </div>
            <div
              class="truncate text-admin-caption font-mono text-admin-subtitle-color"
              :title="card.providerEndpoint.baseUrl"
            >
              {{ card.providerEndpoint.baseUrl }}
            </div>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            class="inline-flex h-6 items-center gap-1.5 px-2.5 rounded-full text-xs font-semibold leading-none border select-none shrink-0 cursor-pointer transition-colors focus-visible:outline-none"
            :class="
              card.providerEndpoint.status === 1
                ? 'bg-admin-success-soft text-admin-success border-admin-success/30 shadow-xs hover:border-admin-success/60'
                : 'bg-admin-surface-muted text-admin-text-3 border-admin-border-soft hover:border-admin-border'
            "
            :title="card.providerEndpoint.status === 1 ? '点击停用端点' : '点击启用端点'"
            :aria-label="card.providerEndpoint.status === 1 ? '点击停用端点' : '点击启用端点'"
            @click.stop="
              commands.toggleProviderEndpointStatus({
                kind: 'providerEndpoint',
                id: card.providerEndpoint.id,
              })
            "
          >
            <span
              class="size-1.5 rounded-full"
              :class="
                card.providerEndpoint.status === 1
                  ? 'bg-admin-success animate-pulse'
                  : 'bg-admin-text-3'
              "
            />
            {{ card.providerEndpoint.status === 1 ? '启用' : '停用' }}
          </button>
        </div>
      </div>
    </template>

    <template #content>
      <div class="flex flex-col gap-3.5">
        <!-- 结构化指标栏 (Sleek Metric Strip) -->
        <div
          class="flex items-center justify-between gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted px-3 py-2 text-xs"
        >
          <div class="flex items-center gap-2 truncate">
            <span class="font-semibold text-admin-title-color">
              {{ card.providerEndpoint.protocolLabel }}
            </span>
            <span
              v-if="isImagePipelineProtocol(card.providerEndpoint.protocol)"
              class="text-admin-text-3"
            >
              ·
              {{ card.providerEndpoint.capabilities[0] }}
            </span>
          </div>

          <div class="flex shrink-0 items-center gap-2">
            <span
              v-if="isImagePipelineProtocol(card.providerEndpoint.protocol)"
              :class="
                card.providerEndpoint.apiKeyConfigured
                  ? 'text-admin-success font-medium'
                  : 'text-admin-warning font-medium'
              "
            >
              {{ card.providerEndpoint.apiKeyConfigured ? 'API Key 已配置' : '未配置 Key' }}
            </span>
            <template v-else-if="card.providerEndpoint.protocol === 'model-api'">
              <span class="font-medium text-admin-text-2">
                🔑 {{ card.accounts.length }} Key
              </span>
              <span class="text-admin-border">|</span>
              <span class="font-medium text-admin-text-2"> ⚡ {{ card.modelCount }} 模型 </span>
            </template>
          </div>
        </div>

        <!-- Model API 关联 APIKey 与模型管理主体 -->
        <div
          v-if="card.providerEndpoint.protocol === 'model-api'"
          class="flex flex-col gap-2.5"
        >
          <!-- 扁平化 Account / APIKey 列表 -->
          <div v-if="card.accounts.length > 0" class="flex flex-col gap-2">
            <div
              v-for="account in card.accounts"
              :key="account.id"
              class="flex flex-col gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted/40 p-2.5 transition-colors hover:border-admin-border"
            >
              <!-- Account 头部 (点击整栏进入编辑，2K/4K 胶囊并列于 0/3 旁) -->
              <div class="flex items-center justify-between gap-2">
                <button
                  type="button"
                  class="group flex min-w-0 w-full items-center justify-between gap-2 border-0 bg-transparent p-0 text-left cursor-pointer focus-visible:outline-none"
                  title="编辑 APIKey"
                  aria-label="编辑 APIKey"
                  @click="accountEditor.openEdit(account.commandTarget)"
                >
                  <div class="flex min-w-0 items-center gap-1.5">
                    <span
                      class="size-1.5 shrink-0 rounded-full"
                      :class="commands.getAccountStatusDotClass(account)"
                    />
                    <span
                      class="truncate text-xs font-semibold text-admin-title-color transition-colors group-hover:text-admin-accent"
                    >
                      {{ account.name || 'APIKey' }}
                    </span>
                  </div>

                  <div class="flex shrink-0 items-center gap-1">
                    <span
                      v-if="account.models.some((m) => m.supports2K)"
                      class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-2"
                    >
                      2K
                    </span>
                    <span
                      v-if="account.models.some((m) => m.supports4K)"
                      class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-2"
                    >
                      4K
                    </span>
                    <span
                      class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-3"
                    >
                      {{ account.concurrencyLabel }}
                    </span>
                  </div>
                </button>
              </div>

              <!-- 并发占用细进度条 (有并发时显示) -->
              <ProgressBar
                v-if="account.concurrencyPercent > 0"
                :value="account.concurrencyPercent"
                :show-value="false"
                class="!h-[2px] !rounded-full !bg-admin-border-soft"
                :pt="commands.getAccountProgressPt(account)"
              />

              <!-- 模型列表 (极简无冗余) -->
              <div class="flex flex-col gap-1">
                <div
                  v-for="model in commands.getAccountModelRows(account)"
                  :key="`${account.id}:${model.name}`"
                  class="flex items-center justify-between gap-2 rounded-lg border border-admin-border-soft/70 bg-white px-2.5 py-1.5 transition-colors hover:border-admin-border"
                >
                  <div class="min-w-0 flex-1 flex items-center gap-2">
                    <span
                      class="truncate text-xs font-medium"
                      :class="model.disabled ? 'text-admin-text-3' : 'text-admin-text-1'"
                    >
                      {{ model.name }}
                    </span>
                  </div>
                  <ToggleSwitch
                    :model-value="!model.disabled"
                    size="small"
                    class="!shrink-0"
                    :aria-label="`${model.name} 模型启用状态`"
                    :disabled="
                      commands.isAccountModelStatusChanging(account.commandTarget, model.name)
                    "
                    @update:model-value="
                      commands.toggleAccountModelStatus(account.commandTarget, model.name)
                    "
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- 常驻虚线添加按钮（更紧凑的极简 + 号） -->
          <button
            type="button"
            class="group flex h-8.5 w-full items-center justify-center rounded-xl border border-dashed border-admin-border bg-transparent cursor-pointer transition-colors hover:border-admin-accent hover:bg-admin-surface-muted/60 focus-visible:outline-none focus-visible:border-admin-accent"
            title="添加 APIKey"
            aria-label="添加 APIKey"
            @click="accountEditor.openCreate(card.providerEndpoint.commandTarget)"
          >
            <Plus
              class="size-4 text-admin-text-3 transition-colors group-hover:text-admin-accent"
            />
          </button>
        </div>

        <!-- 底部端点操作栏 (编辑与删除) -->
        <div class="mt-auto flex items-center justify-between gap-2 pt-1">
          <div class="flex items-center gap-2">
            <Button
              size="small"
              label="编辑"
              severity="contrast"
              @click="providerEndpointEditor.openEdit(card.providerEndpoint.commandTarget)"
            />
            <Button
              size="small"
              label="删除"
              severity="contrast"
              outlined
              @click="commands.deleteProviderEndpoint(card.providerEndpoint.commandTarget)"
            />
          </div>
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Card from 'primevue/card'
import ProgressBar from 'primevue/progressbar'
import ToggleSwitch from 'primevue/toggleswitch'
import { Plus } from '@iconoir/vue'
import { adminCardPt } from '../shared/lib/adminCardPt'
import {
  adminLogoButtonClass,
  adminMasonryCardClass,
} from '../shared/lib/adminUiClasses'
import type { ProviderEndpointCard } from './providerConfigTypes'
import type { ProviderEndpointEditor } from './providerEndpointEditor'
import type { AccountEditor } from './accountEditor'
import type { AdminProviderConfigCommands } from './useAdminProviderConfig'
import { isImagePipelineProtocol } from './providerConfigPayload'
import { getProviderEndpointLogo } from './providerLogos'

defineProps<{
  card: ProviderEndpointCard
  commands: AdminProviderConfigCommands
  providerEndpointEditor: ProviderEndpointEditor
  accountEditor: AccountEditor
  activeLogoTargetId?: number | undefined
}>()

const emit = defineEmits<{
  'open-logo-picker': [event: Event, card: ProviderEndpointCard]
}>()
</script>
