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
            :input-label="PROMPTS_MESSAGES.searchLabel"
            :placeholder="PROMPTS_MESSAGES.searchPlaceholder"
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            all-value="all"
            :label="PROMPTS_MESSAGES.filterLabel"
          />
        </div>

        <!-- 右侧：刷新操作 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton :loading="loading" :aria-label="PROMPTS_MESSAGES.refresh" @click="loadPrompts()" />
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个工作流模块 -->
      <div class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1">
            {{ PROMPTS_MESSAGES.moduleCount(filteredFeatureGroups.length) }}
          </span>
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            {{ PROMPTS_MESSAGES.totalCount(featureGroups.length) }}
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 错误告警状态 -->
    <div v-if="loadFailed && prompts.length === 0" :class="adminEmptyStateClass" role="alert">
      <div class="text-admin-title font-semibold text-admin-title-color">{{ PROMPTS_MESSAGES.loadFailedTitle }}</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">
        {{ PROMPTS_MESSAGES.loadFailedHint }}
      </div>
      <AdminButton class="mt-4" variant="outline" :disabled="loading" @click="loadPrompts()">
        <Refresh class="size-4" />
        {{ PROMPTS_MESSAGES.reload }}
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
        {{ !hasActiveFilter ? PROMPTS_MESSAGES.emptyNoConfig : PROMPTS_MESSAGES.emptyNoMatch }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-admin-text-3">
        {{
          !hasActiveFilter
            ? PROMPTS_MESSAGES.emptyNoConfigHint
            : PROMPTS_MESSAGES.emptyNoMatchHint
        }}
      </p>
      <button
        v-if="hasActiveFilter"
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        @click="clearFilters"
      >
        <Xmark class="size-3.5" />
        <span>{{ PROMPTS_MESSAGES.clearFilters }}</span>
      </button>
    </div>

    <!-- 4. 分组表格视图 (按 Image / Prompt / Fusion 分组展示，1:1 对齐功能定价页 BillingPricingTable 规范) -->
    <div v-else class="flex flex-col gap-6">
      <section
        v-for="group in filteredFeatureGroups"
        :key="group.key"
        :aria-label="PROMPTS_MESSAGES.groupAria(group.title)"
      >
        <!-- 分组标题栏 (1:1 对齐 BillingPricingTable section header) -->
        <div class="mb-2 flex items-center gap-2">
          <h3 class="text-sm font-bold text-admin-title-color">{{ group.title }}</h3>
          <span class="text-xs text-admin-text-3">{{ PROMPTS_MESSAGES.featureCount(group.items.length) }}</span>
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
              <div class="p-6 text-center text-xs text-admin-text-3">{{ PROMPTS_MESSAGES.groupEmpty }}</div>
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
                    <span class="truncate text-admin-caption text-admin-text-3">{{
                      group.title
                    }}</span>
                  </div>
                </div>
              </template>
            </AdminColumn>

            <!-- 2. Status (1:1 对齐功能定价页 Status 规范) -->
            <AdminColumn field="status" header="Status" :style="{ width: '105px' }">
              <template #body="{ data }">
                <span
                  :class="[
                    'inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-medium leading-none',
                    rowView(data.slug).metric.activePillClass,
                  ]"
                >
                  <span :class="['size-1.5 rounded-full', rowView(data.slug).metric.dotClass]" />
                  <span>{{ rowView(data.slug).configured ? PROMPTS_MESSAGES.configured : PROMPTS_MESSAGES.unconfigured }}</span>
                </span>
              </template>
            </AdminColumn>

            <!-- 3. 可用变量 (Variables) -->
            <AdminColumn
              field="variables"
              :header="PROMPTS_MESSAGES.variablesColumn"
              :style="{ width: '220px', minWidth: '180px' }"
            >
              <template #body="{ data }">
                <div class="flex flex-wrap items-center gap-1">
                  <span
                    v-for="variable in rowView(data.slug).vars"
                    :key="variable.key"
                    class="inline-flex items-center rounded bg-admin-surface-muted/80 px-1.5 py-0.5 font-mono text-admin-micro font-medium text-admin-text-2"
                  >
                    {{ variable.key }}
                  </span>
                  <span
                    v-if="rowView(data.slug).vars.length === 0"
                    class="text-xs text-admin-text-3 font-mono"
                  >
                    {{ PROMPTS_MESSAGES.noVariables }}
                  </span>
                </div>
              </template>
            </AdminColumn>

            <!-- 4. 提示词内容预览 (Prompt Preview, 只读已保存值, draft 仅弹窗内可见) -->
            <AdminColumn field="prompt" :header="PROMPTS_MESSAGES.previewColumn">
              <template #body="{ data }">
                <div
                  v-if="rowView(data.slug).saved?.trim()"
                  class="line-clamp-1 max-w-xl font-mono text-xs text-admin-text-2"
                  :title="rowView(data.slug).saved"
                >
                  {{ rowView(data.slug).saved }}
                </div>
                <span v-else class="text-xs text-admin-text-3 font-mono">{{ PROMPTS_MESSAGES.unset }}</span>
              </template>
            </AdminColumn>

            <!-- 5. 操作 (Actions, 1:1 对齐功能定价表规范) -->
            <AdminColumn field="actions" :header="PROMPTS_MESSAGES.actionsColumn" :style="{ width: '120px' }">
              <template #body="{ data }">
                <div class="flex items-center gap-2">
                  <AdminButton
                    size="small"
                    :class="adminActionButtonClass"
                    :aria-label="PROMPTS_MESSAGES.configureAria(data.title)"
                    :disabled="isInitialLoading"
                    @click="openEditor(data)"
                  >
                    {{ rowView(data.slug).configured ? PROMPTS_MESSAGES.adjust : PROMPTS_MESSAGES.setup }}
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
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import {
  featureMap,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureItem,
  type FeatureSlug,
} from '../featurePromptMeta'
import { PROMPTS_MESSAGES } from './messages'

const feedback = usePromptConfigFeedback()
const promptConfigWorkspace = useAdminPromptConfigWorkspace({
  feedback,
  confirm: useAdminConfirm(),
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
  return PROMPTS_MESSAGES.editorHeader(formatServiceName(activeItem.value.slug))
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
