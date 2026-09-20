<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="isListError" severity="error">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>模型库加载失败，请重试。</span>
        <AdminButton
          variant="outline"
          size="small"
          :class="adminActionButtonClass"
          :disabled="isLoading"
          @click="refetchCatalog"
        >
          重试
        </AdminButton>
      </div>
    </AdminAlert>
    <AdminAlert v-else-if="isListStale" severity="warning">
      模型库后台刷新失败，当前显示上次成功数据。
    </AdminAlert>

    <!-- 1. 顶栏筛选器与搜索区 (1:1 复刻 Cloudflare AI Models 交互，统一使用项目 Token) -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索模型输入框 + 维度筛选器下拉 (任务类型、提供商、托管类型) -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            input-label="搜索模型"
            placeholder="搜索模型名称、标识或提供商..."
          />

          <!-- 筛选器：模型 mode / 提供商 / 托管方式（mode 取数据去重，无预定义分类） -->
          <AdminModelFilter
            v-model="selectedMode"
            label="模型类型"
            all-label="全部类型"
            :options="modeOptions"
          />
          <AdminModelFilter
            v-model="selectedProvider"
            label="提供商"
            all-label="全部提供商"
            :options="providerOptions"
          />
          <AdminModelFilter
            v-model="selectedHosting"
            label="托管方式"
            all-label="全部托管"
            :options="hostingOptions"
          />
        </div>

        <!-- 右侧：刷新 + 新增模型 (主操作按钮) -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton :loading="isRefreshing" aria-label="刷新模型" @click="handleRefresh" />

          <!-- 新增网关按钮 -->
          <button
            type="button"
            class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
            aria-label="新增网关"
            @click="gatewayDialogVisible = true"
          >
            <Globe class="size-3.5 text-admin-text-2" />
            <span>新增网关</span>
          </button>

          <!-- 主按钮：添加模型 (使用项目品牌紫 Token) -->
          <button
            type="button"
            class="flex h-9 items-center gap-1.5 rounded-lg bg-admin-accent px-3.5 text-xs font-semibold text-white transition-colors hover:bg-admin-accent-hover cursor-pointer"
            aria-label="添加模型"
            @click="handleCreateModel"
          >
            <Plus class="size-4" />
            <span>添加模型</span>
          </button>
        </div>
      </div>

      <!-- 次级统计提示条：我们找到了 X 个模型 (Cloudflare 1:1 原生文案与布局) -->
      <div class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1">我们找到了 {{ filteredModels.length }} 个模型</span>
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            (共 {{ allModels.length }} 个)
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
        :key="model.id"
        class="group flex flex-col h-full overflow-hidden rounded-xl border border-admin-border-soft bg-white transition-colors hover:border-admin-accent/50"
      >
        <!-- 卡片主体区域 (Upper Section) -->
        <AdminModelCardBody :model="model" hoverable />
        <!-- 卡片底栏 (1:1 复刻 Cloudflare ModelCard Secondary Footer: 条款/文档链接 + 复制 ID / 编辑 / 删除 按钮) -->
        <div class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg/50 px-4 py-2 text-xs">
          <!-- 左侧：条款/文档外部链接 (若无则留白) -->
          <div class="flex items-center">
            <a
              v-if="model.termsUrl"
              :href="model.termsUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1 text-[11px] font-medium text-admin-text-3 hover:text-admin-accent hover:underline transition-colors"
            >
              <span>条款与文档</span>
              <OpenNewWindow class="size-3" />
            </a>
          </div>

          <!-- 右侧：复制 ID / 编辑 / 删除 按钮 -->
          <div class="flex items-center gap-1">
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs font-medium text-admin-text-2 hover:bg-white hover:text-admin-text-1 cursor-pointer transition-colors"
              :aria-label="`复制 ${model.id} 标识`"
              @click="copyModelId(model.id)"
            >
              <Check v-if="copiedModelId === model.id" class="size-3.5 text-emerald-600" />
              <Copy v-else class="size-3.5 text-admin-text-3" />
              <span :class="copiedModelId === model.id ? 'text-emerald-600 font-semibold' : ''">
                {{ copiedModelId === model.id ? '已复制' : '复制' }}
              </span>
            </button>

            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs font-medium text-admin-text-2 hover:bg-white hover:text-admin-text-1 cursor-pointer transition-colors"
              :aria-label="`编辑 ${model.name}`"
              @click="handleEditModel(model)"
            >
              <EditPencil class="size-3.5 text-admin-text-3" />
              <span>编辑</span>
            </button>

            <button
              type="button"
              class="inline-flex items-center justify-center rounded-md p-1 text-xs font-medium text-admin-text-3 hover:bg-white hover:text-admin-danger cursor-pointer transition-colors"
              :aria-label="`删除 ${model.name}`"
              @click="handleDeleteModel(model)"
            >
              <Trash class="size-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. 空状态提示 (空库或筛选无结果时) -->
    <div
      v-else
      class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-admin-border-soft bg-white p-10 text-center"
    >
      <div class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-admin-surface-muted text-admin-text-3">
        <Cube v-if="!hasActiveFilter && allModels.length === 0" class="size-6" />
        <Search v-else class="size-6" />
      </div>
      <h3 class="text-base font-semibold text-admin-title-color">
        {{ !hasActiveFilter && allModels.length === 0 ? '模型库暂无模型' : '未找到匹配的模型' }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-admin-text-3">
        {{
          !hasActiveFilter && allModels.length === 0
            ? '当前模型库为空，请点击下方或右上角按钮自动探测网关模型并一键入库。'
            : '未找到符合当前条件的模型。请尝试调整搜索关键词或重置筛选器。'
        }}
      </p>
      <button
        v-if="!hasActiveFilter && allModels.length === 0"
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg bg-admin-accent px-3.5 text-xs font-semibold text-white transition-colors hover:bg-admin-accent-hover cursor-pointer"
        aria-label="从空状态添加模型"
        @click="handleCreateModel"
      >
        <Plus class="size-3.5" />
        <span>立即添加模型</span>
      </button>
      <button
        v-else
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        @click="clearFilters"
      >
        <Xmark class="size-3.5" />
        <span>清空筛选条件</span>
      </button>
    </div>

    <!-- 4. 添加模型弹窗 (自动探测 + 极简多选入库) -->
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

    <!-- 6. 新增/配置网关弹窗 -->
    <AdminGatewayConfigDialog
      v-model:visible="gatewayDialogVisible"
    />
  </div>
</template>

<script setup lang="ts">
import {
  Check,
  Copy,
  Cube,
  EditPencil,
  Globe,
  OpenNewWindow,
  Plus,
  Search,
  Trash,
  Xmark,
} from '@iconoir/vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import {
  adminActionButtonClass,
  adminDashboardRootClass,
} from '../../../shared/lib/uiClasses'
import AdminModelAddDialog from '../dialog/AdminModelAddDialog.vue'
import AdminModelEditDialog from '../dialog/AdminModelEditDialog.vue'
import AdminGatewayConfigDialog from '../dialog/AdminGatewayConfigDialog.vue'
import AdminModelFilter from '../controls/AdminModelFilter.vue'
import AdminModelCardBody from '../controls/AdminModelCardBody.vue'
import AdminToolbarSearch from '../../../shared/components/AdminToolbarSearch.vue'
import AdminRefreshButton from '../../../shared/components/AdminRefreshButton.vue'
import { useAdminModelCatalogWorkspace } from '../../application/useAdminModelCatalogWorkspace'

// Page 只做布局与绑定：目录用例编排归 workspace，query key 归 workspace 所有。
const {
  allModels,
  isLoading,
  isListError,
  isListStale,
  refetchCatalog,
  isSaving,
  isRefreshing,
  addDialogVisible,
  editDialogVisible,
  editingModel,
  gatewayDialogVisible,
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
