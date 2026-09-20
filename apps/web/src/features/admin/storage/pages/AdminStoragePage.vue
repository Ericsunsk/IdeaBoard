<template>
  <div class="flex flex-col gap-4">
    <!-- 1. 顶栏操作与搜索过滤工具条 (1:1 复刻功能定价页规范) -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框与状态筛选器 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            input-label="搜索存储对象"
            placeholder="搜索存储名称、供应商或存储桶..."
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            all-value="all"
            label="筛选存储状态"
          />
        </div>

        <!-- 右侧：刷新同步与新增操作 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton :loading="loading" aria-label="刷新存储" @click="refreshData" />

          <AdminButton
            :class="adminActionButtonClass"
            :disabled="creatableProviders.length === 0"
            @click="handleOpenCreate"
          >
            <Plus class="size-4" />
            新增存储
          </AdminButton>
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个对象存储 -->
      <div v-if="!loading" class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1">
            共 {{ filteredStorageRows.length }} 个存储服务
          </span>
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            (共 {{ allStorageRows.length }} 个)
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 加载中状态 (AdminSpinner 自带 role="progressbar") -->
    <div
      v-if="loading && storages.length === 0"
      class="flex min-h-[260px] items-center justify-center rounded-xl border border-admin-border-soft bg-white text-admin-text-3"
      aria-label="加载存储配置中"
    >
      <AdminSpinner class="size-9 text-admin-accent" />
    </div>

    <!-- 3. 空状态提示 (无数据或过滤未命中) -->
    <div
      v-else-if="filteredStorageRows.length === 0"
      class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-admin-border-soft bg-white p-10 text-center"
    >
      <div
        class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-admin-surface-muted text-admin-text-3"
      >
        <DatabaseExport v-if="!hasActiveFilter" class="size-6" />
        <Search v-else class="size-6" />
      </div>
      <h3 class="text-base font-semibold text-admin-title-color">
        {{ !hasActiveFilter ? '暂无存储配置' : '未找到匹配的存储配置' }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-admin-text-3">
        {{
          !hasActiveFilter
            ? '当前没有已注册的对象存储配置。点击右上角新增存储开始配置。'
            : '未找到符合当前搜索或筛选条件的存储。请尝试调整关键词或清空筛选。'
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

    <!-- 4. 存储配置表格 (1:1 对齐功能定价页 BillingPricingTable 规范，预设厂商平铺展示) -->
    <section v-else aria-label="对象存储">
      <div class="mb-2 flex items-center gap-2">
        <h3 class="text-sm font-bold text-admin-title-color">对象存储</h3>
        <span class="text-xs text-admin-text-3">共 {{ filteredStorageRows.length }} 个配置</span>
      </div>

      <div :class="adminTableCardClass">
        <AdminTable
          :value="filteredStorageRows"
          :loading="loading"
          data-key="id"
          scrollable
          :frame-class="adminTableFrameClass"
          :table-style="adminTableStyle"
          :class="adminTableClass"
          table-aria-label="对象存储配置列表"
          scroll-aria-label="对象存储配置列表横向滚动区域"
        >
          <template #empty>
            <AdminEmptyState title="暂无匹配存储" />
          </template>

          <!-- 存储服务列 (图标/Logo + 名称 + 供应商标签) -->
          <AdminColumn
            field="service"
            header="存储服务"
            :style="{ width: '240px', minWidth: '200px' }"
          >
            <template #body="{ data }">
              <div class="flex min-w-0 items-center gap-2.5">
                <button
                  v-if="!data.isPresetOnly && data.storage"
                  type="button"
                  :class="[adminLogoButtonClass, 'size-8 p-1.5 shrink-0 rounded-lg shadow-xs']"
                  :data-control-state="
                    activeLogoTarget?.id === data.storage.id ? 'selected' : 'idle'
                  "
                  :aria-expanded="activeLogoTarget?.id === data.storage.id ? 'true' : 'false'"
                  title="点击切换存储 Logo"
                  aria-label="切换存储 Logo"
                  @click="openLogoPicker($event, data)"
                >
                  <img
                    v-if="data.vendorLogo"
                    :src="data.vendorLogo"
                    :alt="data.name"
                    class="size-full object-contain"
                  />
                  <component
                    :is="data.vendorIcon"
                    v-else-if="data.vendorIcon"
                    class="size-4 text-admin-text-2"
                  />
                  <span
                    v-else
                    class="text-[11px] font-bold text-admin-accent uppercase select-none"
                  >
                    {{ data.avatarLabel }}
                  </span>
                </button>
                <div
                  v-else
                  class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-white p-1.5 shadow-xs"
                >
                  <img
                    v-if="data.vendorLogo"
                    :src="data.vendorLogo"
                    :alt="data.name"
                    class="size-full object-contain opacity-80"
                  />
                  <component
                    :is="data.vendorIcon"
                    v-else-if="data.vendorIcon"
                    class="size-4 text-admin-text-3"
                  />
                  <span
                    v-else
                    class="text-[11px] font-bold text-admin-text-3 uppercase select-none"
                  >
                    {{ data.avatarLabel }}
                  </span>
                </div>
                <div class="flex min-w-0 flex-col">
                  <span
                    class="truncate text-xs font-bold"
                    :class="data.isPresetOnly ? 'text-admin-text-2' : 'text-admin-title-color'"
                  >
                    {{ data.name }}
                  </span>
                  <span class="truncate text-[11px] font-medium text-admin-text-3">{{
                    data.subtitle
                  }}</span>
                </div>
              </div>
            </template>
          </AdminColumn>

          <!-- 状态列 (对齐 BillingStatusCell 胶囊 + 圆点规范) -->
          <AdminColumn field="status" header="状态" :style="{ width: '110px' }">
            <template #body="{ data }">
              <span
                v-if="data.isCurrent"
                class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-none bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200"
              >
                <span class="size-1.5 rounded-full bg-emerald-500" />
                <span>使用中</span>
              </span>
              <span
                v-else-if="data.statusType === 'configured'"
                class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-none bg-sky-50 text-sky-700 ring-1 ring-sky-200"
              >
                <span class="size-1.5 rounded-full bg-sky-500" />
                <span>已配置</span>
              </span>
              <span
                v-else
                class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-none bg-slate-100 text-slate-600 ring-1 ring-slate-200"
              >
                <span class="size-1.5 rounded-full bg-slate-400" />
                <span>未配置</span>
              </span>
            </template>
          </AdminColumn>

          <!-- 存储桶 / 路径 -->
          <AdminColumn field="bucket" header="存储桶 / 路径" :style="{ width: '180px' }">
            <template #body="{ data }">
              <span
                class="truncate font-mono text-xs"
                :class="data.bucket === '—' ? 'text-admin-text-3' : 'text-admin-text-1'"
              >
                {{ data.bucket }}
              </span>
            </template>
          </AdminColumn>

          <!-- 访问端点 / 域名 -->
          <AdminColumn field="endpoint" header="访问端点 / 域名" :style="{ width: '220px' }">
            <template #body="{ data }">
              <span
                class="truncate font-mono text-xs"
                :class="data.isPresetOnly ? 'text-admin-text-3' : 'text-admin-text-2'"
                :title="data.endpoint"
              >
                {{ data.endpoint }}
              </span>
            </template>
          </AdminColumn>

          <!-- 当前生效状态控制 -->
          <AdminColumn field="current" header="当前生效" :style="{ width: '110px' }">
            <template #body="{ data }">
              <span
                v-if="data.isCurrent"
                class="inline-flex items-center gap-1 font-mono text-xs font-semibold text-admin-success"
              >
                <Check class="size-3.5" />
                当前生效
              </span>
              <AdminButton
                v-else-if="!data.isPresetOnly && data.storage"
                size="small"
                variant="outline"
                :class="adminActionButtonClass"
                :loading="applyingId === data.storage.id"
                :disabled="!data.canApply || applyingId !== null"
                @click="applyStorage(data.storage)"
              >
                设为当前
              </AdminButton>
              <span v-else class="text-xs font-mono text-admin-text-3">—</span>
            </template>
          </AdminColumn>

          <!-- 操作列 (未配置显示“配置”，已配置显示“编辑/测试/删除”) -->
          <AdminColumn field="actions" header="操作" :style="{ width: '160px' }">
            <template #body="{ data }">
              <div v-if="data.isPresetOnly" class="flex items-center gap-1.5">
                <AdminButton
                  size="small"
                  :class="adminActionButtonClass"
                  @click="handleConfigurePreset(data.presetKey)"
                >
                  配置
                </AdminButton>
              </div>
              <div v-else-if="data.storage" class="flex items-center gap-1.5">
                <AdminButton
                  size="small"
                  variant="outline"
                  :class="adminActionButtonClass"
                  @click="handleOpenEdit(data.storage.id)"
                >
                  编辑
                </AdminButton>
                <AdminButton
                  v-if="data.supportsTest"
                  size="small"
                  variant="outline"
                  :class="adminActionButtonClass"
                  :loading="testingId === data.storage.id"
                  :disabled="!data.canTest || testingId !== null"
                  @click="handleTest(data.storage)"
                >
                  测试连接
                </AdminButton>
                <AdminButton
                  v-if="!data.storage.isCurrent && !data.storage.builtin"
                  size="small"
                  variant="destructive"
                  :class="adminActionButtonClass"
                  @click="requestRemoveProfile(data.storage)"
                >
                  删除
                </AdminButton>
              </div>
            </template>
          </AdminColumn>
        </AdminTable>
      </div>
    </section>
  </div>

  <!-- 存储配置独立弹窗（紧凑表单） -->
  <AdminDialogShell
    :visible="editorOpen"
    :aria-label="editorMode === 'create' ? '添加对象存储' : form.name || '存储配置'"
    description="填写对象存储配置后保存。"
    size="form"
    content-layout="scrollable"
    :busy="saving || savingAndApplying"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full min-w-0 items-center justify-between gap-3 pr-6">
        <div class="flex items-center gap-3">
          <button
            type="button"
            :class="[adminLogoButtonClass, 'size-11 p-2 shrink-0 rounded-xl shadow-xs']"
            :title="editorMode === 'edit' ? '点击切换存储 Logo' : undefined"
            :aria-label="editorMode === 'edit' ? '切换存储 Logo' : undefined"
            :data-control-state="activeLogoTarget?.id === form.id ? 'selected' : 'idle'"
            :aria-expanded="activeLogoTarget?.id === form.id ? 'true' : 'false'"
            @click="editorMode === 'edit' && openLogoPickerForEditing($event)"
          >
            <img
              v-if="getDialogVendorLogo()"
              :src="getDialogVendorLogo()"
              :alt="form.name"
              class="size-full object-contain"
            />
            <component
              :is="vendorIconMap[form.provider] || DatabaseExport"
              v-else
              class="size-5 text-admin-text-2"
            />
          </button>
          <div>
            <h2 class="m-0 text-admin-title-sm font-bold text-admin-title-color">
              {{ editorMode === 'create' ? '添加对象存储' : form.name }}
            </h2>
            <p class="m-0 text-admin-caption text-admin-subtitle-color">
              {{
                editorMode === 'create'
                  ? '选择云存储厂商预设或自定义 S3 兼容接入'
                  : `供应商：${providerLabel(form.provider)}`
              }}
            </p>
          </div>
        </div>

        <AdminBadge v-if="form.isCurrent" value="当前生效存储" severity="success" rounded />
      </div>
    </template>

    <div class="flex flex-col gap-3.5 py-1">
      <!-- 1. 新增模式：顶部极简厂商 Logo 预设卡片选择 -->
      <div v-if="editorMode === 'create'" class="flex items-center gap-2.5 pt-0.5">
        <button
          v-for="vendor in vendorPresetCatalog"
          :key="vendor.key"
          :title="vendor.label"
          type="button"
          class="group relative flex size-12 items-center justify-center rounded-xl border p-2 cursor-pointer transition-colors"
          :class="
            selectedVendorKey === vendor.key
              ? 'border-admin-accent bg-white shadow-xs'
              : 'border-admin-border-soft bg-white hover:border-admin-accent/60'
          "
          :aria-label="vendor.label"
          @click="handleVendorSelect(vendor.key)"
        >
          <img
            :src="vendor.logoSrc"
            :alt="vendor.label"
            class="size-full object-contain transition-transform group-hover:scale-105"
          />
        </button>
      </div>

      <!-- 2. 云厂商多字段分页导航 Tab -->
      <div v-if="textFields.length > 0">
        <nav
          aria-label="存储配置分页"
          class="inline-flex h-9 items-center justify-center rounded-lg bg-admin-surface-muted p-1 text-admin-text-2"
        >
          <button
            v-for="tab in configTabOptions"
            :key="tab.value"
            type="button"
            :class="[
              'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md px-3 py-1 text-admin-caption font-medium transition-colors cursor-pointer',
              activeConfigTab === tab.value
                ? 'bg-white text-admin-text-1 shadow-xs font-semibold'
                : 'hover:text-admin-text-1 text-admin-text-2',
            ]"
            @click="activeConfigTab = tab.value"
          >
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- 3. Tab 1: 基础与凭据 (或本地存储默认视图) -->
      <template v-if="textFields.length === 0 || activeConfigTab === 'basic'">
        <!-- 启用状态开关卡片 -->
        <div
          class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
        >
          <span class="text-admin-body font-semibold text-admin-title-color">启用该存储配置</span>
          <div class="flex items-center gap-2.5">
            <span
              class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-semibold"
              :class="
                form.enabled
                  ? 'bg-admin-success-soft text-admin-success'
                  : 'bg-admin-surface-muted text-admin-text-3 border border-admin-border-soft'
              "
            >
              <span
                class="size-1.5 rounded-full"
                :class="form.enabled ? 'bg-admin-success animate-pulse' : 'bg-admin-text-3'"
              />
              {{ form.enabled ? '已启用' : '已关闭' }}
            </span>
            <AdminSwitch v-model="form.enabled" aria-label="存储启用状态" />
          </div>
        </div>

        <!-- 存储名称 -->
        <AdminFormField label="存储配置名称" for="admin-storage-name" required>
          <AdminInput
            id="admin-storage-name"
            v-model="form.name"
            class="w-full !text-xs"
            placeholder="例如：生产环境主存储 / 阿里云 OSS"
          />
        </AdminFormField>

        <!-- Access Key 与 Secret Access Key -->
        <template v-if="textFields.length > 0">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div v-for="field in authFields" :key="field.key" class="flex min-w-0 flex-col gap-1">
              <label
                :for="`admin-storage-${field.key}`"
                class="text-admin-meta font-[680] text-admin-text-2"
              >
                {{ field.label }}
              </label>
              <AdminInput
                :id="`admin-storage-${field.key}`"
                :model-value="configTextValue(field.key)"
                class="w-full !text-xs font-mono"
                :type="field.type === 'password' ? 'password' : 'text'"
                :placeholder="
                  field.secret && form.secretConfigured && !configTextValue(field.key)
                    ? '已配置，留空则保持不变'
                    : field.placeholder || `请输入${field.label}`
                "
                @update:model-value="updateConfigValue(field.key, String($event ?? ''))"
              />
            </div>
          </div>
        </template>
      </template>

      <!-- 4. Tab 2: 端点与网络 -->
      <template v-else-if="activeConfigTab === 'endpoint'">
        <div class="flex flex-col gap-3">
          <div
            v-for="field in endpointFields"
            :key="field.key"
            class="flex min-w-0 flex-col gap-1"
          >
            <div class="flex items-center justify-between">
              <label
                :for="`admin-storage-${field.key}`"
                class="text-admin-meta font-[680] text-admin-text-2"
              >
                {{ field.label }}
              </label>
              <AdminButton
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                text
                size="small"
                :class="[adminTextButtonClass, '!text-xs font-medium']"
                :loading="fetching"
                @click="fetchBucketDomains"
              >
                一键自动探测域名
              </AdminButton>
            </div>
            <AdminInput
              :id="`admin-storage-${field.key}`"
              :model-value="configTextValue(field.key)"
              class="w-full !text-xs font-mono"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="
                field.secret && form.secretConfigured && !configTextValue(field.key)
                  ? '已配置，留空则保持不变'
                  : field.placeholder || `请输入${field.label}`
              "
              @update:model-value="updateConfigValue(field.key, String($event ?? ''))"
            />

            <!-- 探测到的加速域名一键填入 -->
            <div
              v-if="field.key === 'access_domain' && bucketDomains.length > 0"
              class="flex items-center gap-1.5 flex-wrap pt-1"
            >
              <span class="text-[11px] font-semibold text-admin-text-3">探测到域名：</span>
              <button
                v-for="domain in bucketDomains"
                :key="domain"
                type="button"
                class="cursor-pointer rounded-md border border-admin-border-soft bg-white px-2 py-0.5 text-[11px] font-mono font-medium text-admin-accent hover:border-admin-accent transition-colors"
                @click="updateConfigValue(field.key, `https://${domain}`)"
              >
                https://{{ domain }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <!-- 5. Tab 3: 高级选项 -->
      <template v-else-if="activeConfigTab === 'advanced'">
        <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
          <div
            v-for="field in switchFields"
            :key="field.key"
            class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 p-3"
          >
            <div class="flex flex-col">
              <span class="text-xs font-semibold text-admin-title-color">{{ field.label }}</span>
              <span v-if="field.hint" class="text-[11px] text-admin-text-3">{{ field.hint }}</span>
            </div>
            <AdminSwitch
              :model-value="configBoolValue(field.key)"
              @update:model-value="updateConfigValue(field.key, Boolean($event))"
            />
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <AdminButton
        :class="adminActionButtonClass"
        variant="outline"
        :disabled="saving || savingAndApplying"
        @click="closeEditor"
      >
        取消
      </AdminButton>
      <AdminButton
        v-if="!form.isCurrent"
        :class="adminActionButtonClass"
        variant="outline"
        :loading="saving"
        :disabled="savingAndApplying"
        @click="saveDraft"
      >
        保存
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="savingAndApplying"
        :disabled="saving"
        @click="saveAndApply"
      >
        {{ form.isCurrent ? '保存' : '设为当前' }}
      </AdminButton>
    </template>
  </AdminDialogShell>

  <!-- 存储 Logo 选择弹窗 -->
  <AdminLogoPickerPopover
    ref="logoPickerPopoverRef"
    scope="storage-profile"
    :target-id="activeLogoTarget?.id"
    :target-name="activeLogoTarget?.name"
    :target-base-url="activeLogoTarget?.endpoint"
    initial-category="storage"
    @hide="activeLogoTarget = null"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import { Check, DatabaseExport, Plus, Search, Xmark } from '@iconoir/vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminLogoPickerPopover from '../../shared/components/AdminLogoPickerPopover.vue'
import AdminOptionFilter from '../../shared/components/AdminOptionFilter.vue'
import AdminRefreshButton from '../../shared/components/AdminRefreshButton.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import { storageVendorLogo, vendorIconMap, vendorPresetCatalog } from '../meta'
import { useStorageContext } from '../useStorageContext'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import type { StorageCard } from '../profiles'

type StorageConfigTab = 'basic' | 'endpoint' | 'advanced'
const activeConfigTab = ref<StorageConfigTab>('basic')
const configTabOptions = computed<{ label: string; value: StorageConfigTab }[]>(() => {
  const options: { label: string; value: StorageConfigTab }[] = [
    { label: '基础与凭据', value: 'basic' },
    { label: '端点与网络', value: 'endpoint' },
  ]
  if (switchFields.value.length > 0) {
    options.push({ label: '高级选项', value: 'advanced' })
  }
  return options
})
const adminConfirm = useAdminConfirm()
const {
  saving,
  savingAndApplying,
  fetching,
  loading,
  applyingId,
  testingId,
  editorOpen,
  editorMode,
  selectedVendorKey,
  storages,
  bucketDomains,
  form,
  creatableProviders,
  selectedProviderMeta,
  textFields,
  switchFields,
  cards,
  configTextValue,
  configBoolValue,
  updateConfigValue,
  providerLabel,
  handleVendorSelect,
  openCreate,
  openEdit,
  closeEditor,
  saveDraft,
  saveAndApply,
  applyStorage,
  handleTest,
  fetchBucketDomains,
  requestRemoveProfile,
  loadData,
  refreshData,
} = useStorageContext({ confirm: adminConfirm })
const searchKeyword = ref('')

type StatusFilterValue = 'all' | 'current' | 'configured' | 'unconfigured'
const statusFilter = ref<StatusFilterValue>('all')
const statusFilterOptions: { label: string; value: StatusFilterValue }[] = [
  { label: '全部状态', value: 'all' },
  { label: '使用中', value: 'current' },
  { label: '已配置', value: 'configured' },
  { label: '未配置', value: 'unconfigured' },
]

const hasActiveFilter = computed(() => {
  return Boolean(searchKeyword.value.trim()) || statusFilter.value !== 'all'
})

function clearFilters() {
  searchKeyword.value = ''
  statusFilter.value = 'all'
}

function getBucketDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return '/uploads'
  const val = card.storage.config?.bucket_name
  return typeof val === 'string' && val.trim() ? val.trim() : '—'
}

function getEndpointDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return '服务器本地'
  const domain = card.storage.config?.access_domain
  if (typeof domain === 'string' && domain.trim()) return domain.trim()
  const ep = card.storage.config?.endpoint
  if (typeof ep === 'string' && ep.trim()) return ep.trim()
  return '—'
}

const PRESET_MATCHERS: Array<{ key: string; keywords: string[] }> = [
  { key: 'aliyun-oss', keywords: ['aliyuncs.com', 'aliyun', '阿里', 'oss'] },
  { key: 'tencent-cos', keywords: ['myqcloud.com', 'tencent', '腾讯', 'cos'] },
  { key: 'cloudflare-r2', keywords: ['r2.cloudflarestorage.com', 'cloudflare', 'r2'] },
  { key: 'minio', keywords: ['9000', 'minio'] },
  { key: 'aws-s3', keywords: ['amazonaws.com', 's3', 'amazon'] },
]

function matchProfileToPreset(storage: StorageCard): string | null {
  const text = `${storage.config?.endpoint ?? ''} ${storage.name}`.toLowerCase()
  const matched = PRESET_MATCHERS.find((m) => m.keywords.some((k) => text.includes(k)))
  return matched?.key ?? null
}

export interface StorageTableRow {
  id: string
  isPresetOnly: boolean
  presetKey?: string | undefined
  storage?: StorageCard | undefined
  name: string
  subtitle: string
  providerLabel: string
  vendorLogo?: string | undefined
  vendorIcon?: Component | undefined
  avatarLabel?: string | undefined
  isCurrent: boolean
  statusText: '使用中' | '已配置' | '未配置'
  statusType: 'current' | 'configured' | 'unconfigured'
  bucket: string
  endpoint: string
  supportsTest: boolean
  canTest: boolean
  canApply: boolean
}

function buildConfiguredRow(
  card: (typeof cards.items.value)[number],
  presetKey?: string,
): StorageTableRow {
  const isCur = card.storage.isCurrent
  const isConf = card.statusText === '已配置'
  return {
    id: card.storage.id,
    isPresetOnly: false,
    presetKey,
    storage: card.storage,
    name: card.storage.name,
    subtitle: card.subtitle || card.providerLabel,
    providerLabel: card.providerLabel,
    vendorLogo: card.vendorLogo,
    vendorIcon: card.vendorIcon,
    avatarLabel: card.avatarLabel,
    isCurrent: isCur,
    statusText: isCur ? '使用中' : (card.statusText as '使用中' | '已配置' | '未配置'),
    statusType: isCur ? 'current' : isConf ? 'configured' : 'unconfigured',
    bucket: getBucketDisplay(card),
    endpoint: getEndpointDisplay(card),
    supportsTest: card.supportsTest,
    canTest: card.canTest,
    canApply: card.canApply,
  }
}

function buildUnconfiguredPresetRow(preset: (typeof vendorPresetCatalog)[number]): StorageTableRow {
  return {
    id: `preset-${preset.key}`,
    isPresetOnly: true,
    presetKey: preset.key,
    name: preset.label,
    subtitle: preset.description,
    providerLabel: preset.label,
    vendorLogo: preset.logoSrc,
    avatarLabel: preset.label.slice(0, 2),
    isCurrent: false,
    statusText: '未配置',
    statusType: 'unconfigured',
    bucket: '—',
    endpoint: String(preset.defaultConfig?.endpoint ?? '—'),
    supportsTest: false,
    canTest: false,
    canApply: false,
  }
}

const allStorageRows = computed<StorageTableRow[]>(() => {
  const configuredCards = cards.items.value
  const rows: StorageTableRow[] = []
  const claimedProfileIds = new Set<string>()

  // 1. 遍历预设 catalog 中的各大厂商（阿里云 OSS、腾讯云 COS、Amazon S3、Cloudflare R2、MinIO）
  for (const preset of vendorPresetCatalog) {
    const matched = configuredCards.filter(
      (card) =>
        !claimedProfileIds.has(card.storage.id) &&
        matchProfileToPreset(card.storage) === preset.key,
    )

    if (matched.length > 0) {
      for (const card of matched) {
        claimedProfileIds.add(card.storage.id)
        rows.push(buildConfiguredRow(card, preset.key))
      }
    } else {
      rows.push(buildUnconfiguredPresetRow(preset))
    }
  }

  // 2. 将剩余未匹配到特定预设的已配置 profile（例如 local 存储或自定义 S3）追加进来
  for (const card of configuredCards) {
    if (!claimedProfileIds.has(card.storage.id)) {
      rows.push(buildConfiguredRow(card))
    }
  }

  return rows
})

const filteredStorageRows = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  let list = allStorageRows.value

  if (keyword) {
    list = list.filter((r) => {
      const matchName = r.name.toLowerCase().includes(keyword)
      const matchSubtitle = r.subtitle.toLowerCase().includes(keyword)
      const matchBucket = r.bucket.toLowerCase().includes(keyword)
      const matchEndpoint = r.endpoint.toLowerCase().includes(keyword)
      return matchName || matchSubtitle || matchBucket || matchEndpoint
    })
  }

  if (statusFilter.value === 'current') {
    list = list.filter((r) => r.isCurrent)
  } else if (statusFilter.value === 'configured') {
    list = list.filter((r) => r.statusType === 'configured')
  } else if (statusFilter.value === 'unconfigured') {
    list = list.filter((r) => r.statusType === 'unconfigured')
  }

  return list
})

// 分类字段 (按信息密度结构化分类)
const authFields = computed(() =>
  textFields.value.filter(
    (f) => f.key === 'access_key_id' || f.key === 'secret_access_key' || f.secret,
  ),
)
const endpointFields = computed(() =>
  textFields.value.filter(
    (f) => f.key !== 'access_key_id' && f.key !== 'secret_access_key' && !f.secret,
  ),
)

const activeLogoTarget = ref<{ id: string; name: string; endpoint: string } | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function getDialogVendorLogo(): string | undefined {
  if (editorMode.value === 'create') {
    return vendorPresetCatalog.find((v) => v.key === selectedVendorKey.value)?.logoSrc
  }
  return storageVendorLogo(form.value)
}

function openLogoPickerForEditing(event: Event) {
  if (!form.value.id) return
  activeLogoTarget.value = { id: form.value.id, name: form.value.name, endpoint: String(form.value.config?.endpoint || '') }
  logoPickerPopoverRef.value?.toggle(event)
}

function handleOpenCreate() {
  activeConfigTab.value = 'basic'
  openCreate()
}

function handleConfigurePreset(presetKey?: string) {
  activeConfigTab.value = 'basic'
  openCreate()
  if (presetKey) handleVendorSelect(presetKey)
}

function handleOpenEdit(id: string) {
  activeConfigTab.value = 'basic'
  openEdit(id)
}

function openLogoPicker(
  event: Event,
  card: { storage?: { id: string; name: string; config?: Record<string, unknown> } },
) {
  if (!card.storage) return
  activeLogoTarget.value = { id: card.storage.id, name: card.storage.name, endpoint: String(card.storage.config?.endpoint || '') }
  logoPickerPopoverRef.value?.toggle(event)
}

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

onMounted(() => {
  void loadData()
})
</script>
