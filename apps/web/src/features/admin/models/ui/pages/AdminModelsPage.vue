<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="isListStale" severity="warning">
      {{ MODELS_MESSAGES.listStale }}
    </AdminAlert>

    <!-- 1. 顶栏筛选器与搜索区 (1:1 复刻 Cloudflare AI Models 交互，统一使用项目 Token) -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索模型输入框 + 维度筛选器下拉 (任务类型、提供商、托管类型) -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            :input-label="MODELS_MESSAGES.searchLabel"
            :placeholder="MODELS_MESSAGES.searchPlaceholder"
          />

          <!-- 筛选器：模型 mode / 提供商 / 托管方式（mode 取数据去重，无预定义分类） -->
          <AdminModelFilter
            v-model="selectedMode"
            :label="MODELS_MESSAGES.modeLabel"
            :all-label="MODELS_MESSAGES.modeAll"
            :options="modeOptions"
          />
          <AdminModelFilter
            v-model="selectedProvider"
            :label="MODELS_MESSAGES.providerLabel"
            :all-label="MODELS_MESSAGES.providerAll"
            :options="providerOptions"
          />
          <AdminModelFilter
            v-model="selectedHosting"
            :label="MODELS_MESSAGES.hostingLabel"
            :all-label="MODELS_MESSAGES.hostingAll"
            :options="hostingOptions"
          />
        </div>

        <!-- 右侧：刷新 + 新增模型 (主操作按钮) -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton
            :loading="isRefreshing"
            :aria-label="MODELS_MESSAGES.refresh"
            @click="handleRefresh"
          />

          <!-- 主按钮：添加模型 (使用项目品牌紫 Token) -->
          <button
            type="button"
            class="flex h-9 items-center gap-1.5 rounded-control bg-primary px-3.5 text-xs font-semibold text-primary-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-primary-hover cursor-pointer"
            :aria-label="MODELS_MESSAGES.createModel"
            @click="handleCreateModel"
          >
            <Plus class="size-4" />
            <span>{{ MODELS_MESSAGES.createModel }}</span>
          </button>
        </div>
      </div>

      <!-- 次级统计提示条：我们找到了 X 个模型 (Cloudflare 1:1 原生文案与布局) -->
      <div class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-muted-foreground">
          <span class="font-medium text-foreground">{{
            MODELS_MESSAGES.foundCount(filteredModels.length)
          }}</span>
          <span v-if="hasActiveFilter" class="text-xs text-muted-foreground">
            {{ MODELS_MESSAGES.totalCount(allModels.length) }}
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 模型卡片网格列表 (3列响应式网格，1:1 对齐 Cloudflare ModelCardV3 设计规范) -->
    <div
      v-if="filteredModels.length > 0"
      class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3"
    >
      <div
        v-for="model in filteredModels"
        :key="modelIdentityKey(model)"
        class="group flex flex-col h-full overflow-hidden rounded-panel border border-border bg-card transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-primary-line-emphasis"
      >
        <!-- 卡片主体区域 (Upper Section，含近 7 天观测区) -->
        <AdminModelCardBody
          :model="model"
          hoverable
          :observed="observedByModel.get(modelIdentityKey(model))"
          :observed-loading="isObservedLoading"
        />
        <!-- 卡片底栏 (1:1 复刻 Cloudflare ModelCard Secondary Footer: 条款/文档链接 + 复制 ID / 编辑 / 删除 按钮) -->
        <div
          class="flex items-center justify-between border-t border-border bg-background/50 px-4 py-2 text-xs"
        >
          <!-- 左侧：条款/文档外部链接 (若无则留白) -->
          <div class="flex items-center">
            <a
              v-if="model.termsUrl"
              :href="model.termsUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1 text-admin-caption font-medium text-muted-foreground hover:text-primary hover:underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
            >
              <span>{{ MODELS_MESSAGES.termsAndDocs }}</span>
              <OpenNewWindow class="size-3" />
            </a>
          </div>

          <!-- 右侧：复制 ID / 编辑 / 删除 按钮（复制 provider/model 拼接，仅显示） -->
          <div class="flex items-center gap-1">
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-control px-1.5 py-1 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :aria-label="MODELS_MESSAGES.copyIdAria(displayModelId(model))"
              @click="copyModelId(displayModelId(model))"
            >
              <Check v-if="copiedModelId === displayModelId(model)" class="size-3.5 text-success" />
              <Copy v-else class="size-3.5 text-muted-foreground" />
              <span
                :class="copiedModelId === displayModelId(model) ? 'text-success font-semibold' : ''"
              >
                {{
                  copiedModelId === displayModelId(model)
                    ? MODELS_MESSAGES.copied
                    : MODELS_MESSAGES.copy
                }}
              </span>
            </button>

            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-control px-1.5 py-1 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :aria-label="MODELS_MESSAGES.editAria(model.name)"
              @click="handleEditModel(model)"
            >
              <EditPencil class="size-3.5 text-muted-foreground" />
              <span>{{ MODELS_MESSAGES.edit }}</span>
            </button>

            <button
              type="button"
              class="inline-flex items-center justify-center rounded-control p-1 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-destructive cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :aria-label="MODELS_MESSAGES.deleteAria(model.name)"
              @click="handleDeleteModel(model)"
            >
              <Trash class="size-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. 空状态提示 (空库或筛选无结果时) -->
    <AdminEmptyState
      v-else
      :title="
        !hasActiveFilter && allModels.length === 0
          ? MODELS_MESSAGES.emptyLibrary
          : MODELS_MESSAGES.emptyNoMatch
      "
      :description="
        !hasActiveFilter && allModels.length === 0
          ? MODELS_MESSAGES.emptyLibraryHint
          : MODELS_MESSAGES.emptyNoMatchHint
      "
      :icon="!hasActiveFilter && allModels.length === 0 ? Cube : Search"
    >
      <template #actions>
        <button
          v-if="!hasActiveFilter && allModels.length === 0"
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-control bg-primary px-3.5 text-xs font-semibold text-primary-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-primary-hover cursor-pointer"
          :aria-label="MODELS_MESSAGES.emptyCreateAria"
          @click="handleCreateModel"
        >
          <Plus class="size-3.5" />
          <span>{{ MODELS_MESSAGES.createNow }}</span>
        </button>
        <button
          v-else
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted cursor-pointer"
          @click="clearFilters"
        >
          <Xmark class="size-3.5" />
          <span>{{ MODELS_MESSAGES.clearFilters }}</span>
        </button>
      </template>
    </AdminEmptyState>

    <!-- 4. 添加模型弹窗 (手动入库) -->
    <AdminModelAddDialog
      v-model:visible="addDialogVisible"
      :existing-models="allModels"
      :pending="isSaving"
      @import="handleImportModels"
      @create="handleCreateManualModel"
    />

    <!-- 5. 编辑模型弹窗 -->
    <AdminModelEditDialog
      v-model:visible="editDialogVisible"
      :model="editingModel"
      :pending="isSaving"
      @submit="handleSaveModel"
    />
  </div>
</template>

<script setup lang="ts">
import {
  Check,
  Copy,
  Cube,
  EditPencil,
  OpenNewWindow,
  Plus,
  Search,
  Trash,
  Xmark,
} from '@iconoir/vue'
import { AdminAlert } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { adminDashboardRootClass } from '../../../shared'
import AdminModelAddDialog from '../dialog/AdminModelAddDialog.vue'
import AdminModelEditDialog from '../dialog/AdminModelEditDialog.vue'
import AdminModelFilter from '../controls/AdminModelFilter.vue'
import AdminModelCardBody from '../controls/AdminModelCardBody.vue'
import { AdminToolbarSearch } from '../../../shared'
import { AdminRefreshButton } from '../../../shared'
import { MODELS_MESSAGES } from './messages'
import { useAdminModelCatalogWorkspace } from '../../session/useAdminModelCatalogWorkspace'
import { displayModelId, modelIdentityKey } from '../../core/catalogData'

// Page 只做布局与绑定：目录用例编排归 workspace，query key 归 workspace 所有。
const {
  allModels,
  isListStale,
  observedByModel,
  isObservedLoading,
  isSaving,
  isRefreshing,
  addDialogVisible,
  editDialogVisible,
  editingModel,
  searchKeyword,
  selectedMode,
  selectedProvider,
  selectedHosting,
  modeOptions,
  providerOptions,
  hostingOptions,
  hasActiveFilter,
  filteredModels,
  clearFilters,
  handleRefresh,
  handleCreateModel,
  handleCreateManualModel,
  handleImportModels,
  handleEditModel,
  handleDeleteModel,
  handleSaveModel,
  copyModelId,
  copiedModelId,
} = useAdminModelCatalogWorkspace()
</script>
