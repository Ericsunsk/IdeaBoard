<template>
  <AdminDialogShell
    :visible="visible"
    :header="isEditing ? '编辑模型' : '新增模型'"
    description="填写模型配置后保存。"
    size="wide"
    @update:visible="emit('update:visible', $event)"
    @close="emit('update:visible', false)"
  >
    <div class="grid grid-cols-1 gap-6 p-5 lg:grid-cols-12">
      <!-- 左侧：主要配置工作区 (7 列) -->
      <div class="flex flex-col gap-4.5 lg:col-span-7">
        <CatalogIdentityPanel
          v-model:model-id="form.id"
          v-model:model-name="form.name"
          v-model:model-provider="form.provider"
          v-model:model-description="form.description"
          :is-editing="isEditing"
          :id-prefix-options="idPrefixOptions"
          :provider-options="providerOptions"
        />

        <CatalogSpecsPanel
          v-model:context-or-resolution="form.contextOrResolution"
          v-model:hosting="form.hosting"
          v-model:capabilities-draft="capabilitiesDraft"
        />

        <CatalogGatewayPanel
          v-model:custom-path="form.customPath"
          v-model:custom-params="form.customParams"
          :initially-open="showAdvanced"
        />

        <!-- 错误提示 -->
        <div v-if="validationError" class="text-xs text-admin-danger" role="alert">
          {{ validationError }}
        </div>
      </div>

      <!-- 右侧：实时卡片预览与网关透视区 (5 列) -->
      <CatalogPreviewCard
        :model="previewModel"
        :model-id="form.id"
        :route-preview="gatewayRoutePreview"
      />
    </div>

    <!-- 弹窗底栏 -->
    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending"
        @click="emit('update:visible', false)"
      >
        取消
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="pending"
        @click="handleSubmit"
      >
        {{ isEditing ? '保存修改' : '确认新增' }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../../shared/components/AdminDialogShell.vue'
import { adminActionButtonClass } from '../../../shared/lib/uiClasses'
import { createAdminGatewayResource, type GatewayProviderOption } from '../../infrastructure/adminGatewayResource'
import { type AdminModelItem } from '../../domain/catalogData'
import {
  blankModelForm,
  finalizeModelSubmit,
  splitCapabilities,
  validateModelForm,
} from '../../domain/modelForm'
import CatalogIdentityPanel from './components/CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './components/CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './components/CatalogGatewayPanel.vue'
import CatalogPreviewCard, { type GatewayRoutePreviewState } from './components/CatalogPreviewCard.vue'

const props = defineProps<{
  visible: boolean
  model: AdminModelItem | null
  pending?: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  submit: [item: AdminModelItem]
}>()

const catalogResource = createAdminGatewayResource().catalog

const isEditing = computed(() => Boolean(props.model))
const capabilitiesDraft = ref('')
const validationError = ref('')
const showAdvanced = ref(false)

// 供应商 quick-pick 与 ID 前缀全部来自 Registry 接口
const providerOptions = ref<GatewayProviderOption[]>([])
const idPrefixOptions = computed(() => {
  const seen = new Set<string>()
  for (const option of providerOptions.value) {
    if (option.id_prefix) seen.add(option.id_prefix)
  }
  return [...seen]
})

async function loadProviderOptions() {
  if (providerOptions.value.length > 0) return
  try {
    providerOptions.value = await catalogResource.listProviders()
  } catch {
    providerOptions.value = []
  }
}

const defaultForm: AdminModelItem = blankModelForm()
const form = ref<AdminModelItem>({ ...defaultForm })

// 实时预览标签列表
const previewCapabilities = computed(() => {
  const list = splitCapabilities(capabilitiesDraft.value)
  return list.length > 0 ? list : ['标准推理', '云端加速']
})

// 预览用合成模型
const previewModel = computed<AdminModelItem>(() => ({
  id: form.value.id,
  name: form.value.name || '模型名称',
  provider: form.value.provider || '供应商',
  description: form.value.description || '暂无描述。保存后将在前台模型库中完整展示。',
  descriptionEnriched: form.value.descriptionEnriched ?? false,
  contextOrResolution: form.value.contextOrResolution || '2048×2048',
  capabilities: previewCapabilities.value,
  hosting: form.value.hosting,
  termsUrl: '',
  docsUrl: '',
  customPath: form.value.customPath ?? '',
  customParams: form.value.customParams ?? '',
  inputCostPerToken: form.value.inputCostPerToken,
  outputCostPerToken: form.value.outputCostPerToken,
  cacheReadCostPerToken: form.value.cacheReadCostPerToken,
  cacheCreateCostPerToken: form.value.cacheCreateCostPerToken,
  supportsPromptCaching: form.value.supportsPromptCaching,
}))

// 路由透视
const gatewayRoutePreview = ref<GatewayRoutePreviewState>({
  providerSlug: '未指定',
  isFirstParty: false,
  targetPath: '-',
  protocol: '-',
})

async function refreshGatewayRoutePreview() {
  const modelId = form.value.id.trim()
  if (!modelId) {
    gatewayRoutePreview.value = {
      providerSlug: '未指定',
      isFirstParty: false,
      targetPath: '-',
      protocol: '-',
    }
    return
  }
  try {
    const preview = await catalogResource.previewRoute({
      modelId,
      customPath: form.value.customPath?.trim() || undefined,
      customParams: form.value.customParams?.trim() || undefined,
    })
    gatewayRoutePreview.value = {
      providerSlug: preview.provider_slug || '未指定',
      isFirstParty: preview.is_first_party,
      targetPath: preview.target_path,
      protocol: preview.path_source_label || preview.path_source,
    }
  } catch {
    // 预览失败保留上次结果，不干扰表单
  }
}

watchDebounced(
  [() => form.value.id, () => form.value.customPath, () => form.value.customParams],
  () => {
    void refreshGatewayRoutePreview()
  },
  { debounce: 300, immediate: true },
)

watch(
  () => props.visible,
  (val) => {
    if (val) {
      validationError.value = ''
      void loadProviderOptions()
      if (props.model) {
        form.value = { ...props.model }
        capabilitiesDraft.value = props.model.capabilities.join(', ')
        showAdvanced.value = Boolean(props.model.customPath || props.model.customParams)
      } else {
        form.value = { ...defaultForm }
        capabilitiesDraft.value = ''
        showAdvanced.value = false
      }
    }
  },
  { immediate: true },
)

function handleSubmit() {
  validationError.value = ''
  const error = validateModelForm({
    id: form.value.id,
    name: form.value.name,
    provider: form.value.provider,
    customParams: form.value.customParams,
  })
  if (error) {
    validationError.value = error
    return
  }

  emit('submit', finalizeModelSubmit({ ...form.value, descriptionEnriched: false }, splitCapabilities(capabilitiesDraft.value)))
}
</script>