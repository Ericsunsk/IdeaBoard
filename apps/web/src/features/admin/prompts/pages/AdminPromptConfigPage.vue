<template>
  <div :class="adminDashboardRootClass">
    <!-- 1. 顶栏操作与搜索过滤工具条 (1:1 复刻功能定价页规范) -->
    <div v-if="!loadFailed || prompts.length > 0" class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框与状态筛选器 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            input-label="搜索提示词"
            placeholder="搜索功能模块、节点标识或提示词内容..."
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            all-value="all"
            label="筛选提示词状态"
          />
        </div>

        <!-- 右侧：刷新操作 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton :loading="loading" aria-label="刷新提示词" @click="loadPrompts()" />
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个工作流模块 -->
      <div class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1">
            共 {{ filteredFeatureGroups.length }} 个工作流模块
          </span>
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            (共 {{ featureGroups.length }} 个)
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 错误告警状态 -->
    <div v-if="loadFailed && prompts.length === 0" :class="adminEmptyStateClass" role="alert">
      <div class="text-admin-title font-semibold text-admin-title-color">系统提示词加载失败</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">
        未能从服务端加载配置，请重试。
      </div>
      <AdminButton class="mt-4" variant="outline" :disabled="loading" @click="loadPrompts()">
        <Refresh class="size-4" />
        重新加载
      </AdminButton>
    </div>

    <!-- 3. 空状态提示 (无数据或过滤未命中) -->
    <div
      v-else-if="filteredFeatureGroups.length === 0"
      class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-admin-border-soft bg-white p-10 text-center"
    >
      <div
        class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-admin-surface-muted text-admin-text-3"
      >
        <Spark v-if="!hasActiveFilter" class="size-6" />
        <Search v-else class="size-6" />
      </div>
      <h3 class="text-base font-semibold text-admin-title-color">
        {{ !hasActiveFilter ? '暂无系统提示词配置' : '未找到匹配的提示词模块' }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-admin-text-3">
        {{
          !hasActiveFilter
            ? '当前没有可用的工作流节点系统提示词配置。'
            : '未找到符合当前搜索或筛选条件的功能节点。请尝试调整关键词或清空筛选。'
        }}
      </p>
      <button
        v-if="hasActiveFilter"
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        @click="clearFilters"
      >
        <Xmark class="size-3.5" />
        <span>清空筛选条件</span>
      </button>
    </div>

    <!-- 4. 分组表格视图 (按 Image / Prompt / Fusion 分组展示，1:1 对齐功能定价页 BillingPricingTable 规范) -->
    <div v-else class="flex flex-col gap-6">
      <section
        v-for="group in filteredFeatureGroups"
        :key="group.key"
        :aria-label="`${group.title}提示词配置`"
      >
        <!-- 分组标题栏 (1:1 对齐 BillingPricingTable section header) -->
        <div class="mb-2 flex items-center gap-2">
          <h3 class="text-sm font-bold text-admin-title-color">{{ group.title }}</h3>
          <span class="text-xs text-admin-text-3">共 {{ group.items.length }} 个功能</span>
        </div>

        <!-- 数据表格壳体 -->
        <div :class="adminTableCardClass">
          <AdminTable
            :value="group.items"
            :loading="isInitialLoading"
            data-key="slug"
            scrollable
            :frame-class="adminTableFrameClass"
            :table-style="adminTableStyle"
            :class="adminTableClass"
          >
            <template #empty>
              <div class="p-6 text-center text-xs text-admin-text-3">该模块暂无匹配项</div>
            </template>

            <!-- 1. Service (1:1 对齐功能定价页 Service 规范) -->
            <AdminColumn field="service" header="Service" :style="{ width: '200px' }">
              <template #body="{ data }">
                <div class="flex min-w-0 items-center gap-1.5 font-mono text-xs text-admin-text-1">
                  <component
                    :is="getFeatureIcon(data.slug)"
                    class="size-4 shrink-0 text-admin-text-2"
                  />
                  <div class="flex min-w-0 flex-col">
                    <span class="truncate text-slate-700">{{ data.slug }}</span>
                    <span class="truncate text-[11px] text-admin-text-3">{{ group.title }}</span>
                  </div>
                </div>
              </template>
            </AdminColumn>

            <!-- 2. Status (1:1 对齐功能定价页 Status 规范) -->
            <AdminColumn field="status" header="Status" :style="{ width: '105px' }">
              <template #body="{ data }">
                <span
                  :class="[
                    'inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-none',
                    rowView(data.slug).metric.activePillClass,
                  ]"
                >
                  <span :class="['size-1.5 rounded-full', rowView(data.slug).metric.dotClass]" />
                  <span>{{ rowView(data.slug).configured ? '已配置' : '未配置' }}</span>
                </span>
              </template>
            </AdminColumn>

            <!-- 3. 可用变量 (Variables) -->
            <AdminColumn
              field="variables"
              header="可用变量"
              :style="{ width: '220px', minWidth: '180px' }"
            >
              <template #body="{ data }">
                <div class="flex flex-wrap items-center gap-1">
                  <span
                    v-for="variable in rowView(data.slug).vars"
                    :key="variable.key"
                    class="inline-flex items-center rounded bg-admin-surface-muted/80 px-1.5 py-0.5 font-mono text-[10px] font-medium text-admin-text-2"
                  >
                    {{ variable.key }}
                  </span>
                  <span
                    v-if="rowView(data.slug).vars.length === 0"
                    class="text-xs text-admin-text-3 font-mono"
                  >
                    无变量
                  </span>
                </div>
              </template>
            </AdminColumn>

            <!-- 4. 提示词内容预览 (Prompt Preview, 只读已保存值, draft 仅弹窗内可见) -->
            <AdminColumn field="prompt" header="系统提示词预览">
              <template #body="{ data }">
                <div
                  v-if="rowView(data.slug).saved?.trim()"
                  class="line-clamp-1 max-w-xl font-mono text-xs text-admin-text-2"
                  :title="rowView(data.slug).saved"
                >
                  {{ rowView(data.slug).saved }}
                </div>
                <span v-else class="text-xs text-admin-text-3 font-mono">未设置</span>
              </template>
            </AdminColumn>

            <!-- 5. 操作 (Actions, 1:1 对齐功能定价表规范) -->
            <AdminColumn field="actions" header="操作" :style="{ width: '120px' }">
              <template #body="{ data }">
                <div class="flex items-center gap-2">
                  <AdminButton
                    size="small"
                    :class="adminActionButtonClass"
                    :aria-label="`配置 ${data.title} 提示词`"
                    :disabled="isInitialLoading"
                    @click="openEditor(data)"
                  >
                    {{ rowView(data.slug).configured ? '调整' : '设置' }}
                  </AdminButton>
                </div>
              </template>
            </AdminColumn>
          </AdminTable>
        </div>
      </section>
    </div>

    <!-- 5. 提示词编辑弹窗 -->
    <PromptConfigEditorDialog
      :item="activeItem"
      :draft="activeItem ? (promptMap[activeItem.slug] ?? '') : ''"
      :validation="activeValidation"
      :saving="Boolean(activeItem && saving[activeItem.slug])"
      :variables="activeVars"
      :header="editorHeader"
      @update:draft="onDraftInput"
      @save="onEditorSave"
      @close="closeEditor"
      @insert-variable="onInsertVariable"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { Refresh, Search, Spark, Xmark } from '@iconoir/vue'
import { usePromptConfigFeedback } from '../usePromptConfigFeedback'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminOptionFilter from '../../shared/components/AdminOptionFilter.vue'
import AdminRefreshButton from '../../shared/components/AdminRefreshButton.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import PromptConfigEditorDialog from '../components/PromptConfigEditorDialog.vue'
import { usePromptConfigFilters } from '../filtering'
import {
  adminActionButtonClass,
  adminDashboardRootClass,
  adminEmptyStateClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { taskMetricDefinitions } from '../../../../shared/domain/taskMetricTokens'
import { getWorkflowFeatureIcon } from '../../../../shared/domain/workflowFeatureIcons'
import { useAdminPromptConfigWorkspace } from '../workspace'
import type { FeaturePromptVariable } from '../workspace'
import {
  featureMap,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureItem,
  type FeatureSlug,
} from '../featurePromptMeta'

const feedback = usePromptConfigFeedback()
const promptConfigWorkspace = useAdminPromptConfigWorkspace({
  feedback,
  getFeatureTitle: (slug) => featureMap[slug as FeatureSlug]?.title ?? slug,
})
const {
  getDraftValidation,
  getPromptVariables,
  getSavedPrompt,
  insertVariableIntoDraft,
  loadFailed,
  loadPrompts,
  loading,
  promptMap,
  prompts,
  resetDrafts,
  savePrompt,
  saving,
} = promptConfigWorkspace

const activeItem = ref<FeatureItem | null>(null)

function formatServiceName(slug: string): string {
  if (!slug) return ''
  return slug.charAt(0).toUpperCase() + slug.slice(1)
}

const editorHeader = computed(() => {
  if (!activeItem.value) return ''
  return `编辑 · ${formatServiceName(activeItem.value.slug)}`
})

function isConfigured(slug: FeatureSlug): boolean {
  return getSavedPrompt(slug).trim().length > 0
}

const completedMetric =
  taskMetricDefinitions.find((m) => m.key === 'completed') ?? taskMetricDefinitions[0]!
const pendingMetric =
  taskMetricDefinitions.find((m) => m.key === 'pending') ?? taskMetricDefinitions[0]!

function getFeatureIcon(slug: string): Component {
  return getWorkflowFeatureIcon(slug) ?? Spark
}

const featureGroups = computed<FeatureGroup[]>(() =>
  groupedFeatureGroups.filter((group) => group.items.length > 0),
)

const {
  searchKeyword,
  statusFilter,
  statusFilterOptions,
  hasActiveFilter,
  clearFilters,
  filteredFeatureGroups,
} = usePromptConfigFilters({
  groups: featureGroups,
  isConfigured,
  getSavedPrompt,
  getPromptVariables,
})

// 行级 view-model: 模板每行只读一次 map, 避免同一单元格内重复调用
// getSavedPrompt/statusMetricFor/getVarsForSlug (6 行规模下是可读性问题, 非性能瓶颈)。
const rowViewMap = computed(() => {
  const map: Record<
    string,
    {
      saved: string
      configured: boolean
      vars: readonly FeaturePromptVariable[]
      metric: typeof completedMetric
    }
  > = {}
  for (const group of featureGroups.value) {
    for (const item of group.items) {
      const saved = getSavedPrompt(item.slug)
      const configured = saved.trim().length > 0
      map[item.slug] = {
        saved,
        configured,
        vars: getVarsForSlug(item.slug),
        metric: configured ? completedMetric : pendingMetric,
      }
    }
  }
  return map
})

const activeVars = computed<readonly FeaturePromptVariable[]>(() =>
  activeItem.value ? getVarsForSlug(activeItem.value.slug) : [],
)

const activeValidation = computed(() =>
  activeItem.value ? getDraftValidation(activeItem.value.slug) : null,
)

// 模板索引安全：map 恒有全量 slug，fallback 仅满足 noUncheckedIndexedAccess。
const emptyRowView = {
  saved: '',
  configured: false,
  vars: [] as readonly FeaturePromptVariable[],
  metric: pendingMetric,
}

function rowView(slug: string): typeof emptyRowView {
  return rowViewMap.value[slug] ?? emptyRowView
}

function openEditor(item: FeatureItem) {
  resetDrafts([item.slug])
  activeItem.value = item
}

function closeEditor() {
  if (activeItem.value) {
    resetDrafts([activeItem.value.slug])
  }
  activeItem.value = null
}

function onDraftInput(value: string) {
  if (activeItem.value) {
    promptMap[activeItem.value.slug] = value
  }
}

async function onEditorSave() {
  if (!activeItem.value) return
  const result = await savePrompt(activeItem.value.slug)
  if (result) {
    activeItem.value = null
  }
}

function onInsertVariable(payload: { key: string; position: number }) {
  if (!activeItem.value) return
  insertVariableIntoDraft(activeItem.value.slug, payload.key, payload.position)
}

// 初载：数据到达前不渲染 0 值计数，避免闪动误导。
const isInitialLoading = computed(() => loading.value && prompts.value.length === 0)

function getVarsForSlug(slug: FeatureSlug): readonly FeaturePromptVariable[] {
  return getPromptVariables(slug)
}
</script>
