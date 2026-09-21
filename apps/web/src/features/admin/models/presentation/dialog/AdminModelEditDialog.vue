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
          v-model:model-id="form.values.id"
          v-model:model-name="form.values.name"
          v-model:model-provider="form.values.provider"
          v-model:model-description="form.values.description"
          :is-editing="isEditing"
          :id-prefix-options="idPrefixOptions"
          :provider-options="providerOptions"
        />

        <CatalogSpecsPanel
          v-model:context-or-resolution="form.values.contextOrResolution"
          v-model:hosting="form.values.hosting"
          v-model:capabilities-draft="capabilitiesDraft"
        />

        <CatalogGatewayPanel
          v-model:custom-path="form.values.customPath"
          v-model:custom-params="form.values.customParams"
          :initially-open="showAdvanced"
        />

        <!-- 错误提示 -->
        <div v-if="firstValidationError" class="text-xs text-admin-danger" role="alert">
          {{ firstValidationError }}
        </div>
      </div>

      <!-- 右侧：实时卡片预览与网关透视区 (5 列) -->
      <CatalogPreviewCard
        :model="previewModel"
        :model-id="form.values.id"
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
      <AdminButton :class="adminActionButtonClass" :loading="pending" @click="handleSubmit">
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
import {
  createAdminGatewayResource,
  type GatewayProviderOption,
} from '../../infrastructure/adminGatewayResource'
import { type AdminModelItem } from '../../domain/catalogData'
import {
  blankModelForm,
  finalizeModelSubmit,
  modelFormSchema,
  splitCapabilities,
} from '../../domain/modelForm'
import { useForm, firstFormError } from '@/components/ui/form'
import CatalogIdentityPanel from './components/CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './components/CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './components/CatalogGatewayPanel.vue'
import CatalogPreviewCard, {
  type GatewayRoutePreviewState,
} from './components/CatalogPreviewCard.vue'

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

const form = useForm({
  initialValues: { ...blankModelForm() },
  schema: modelFormSchema,
})
const firstValidationError = computed(() =>
  firstFormError(form.errors.value, ['id', 'name', 'provider', 'customParams']),
)

// 实时预览标签列表
const previewCapabilities = computed(() => {
  const list = splitCapabilities(capabilitiesDraft.value)
  return list.length > 0 ? list : ['标准推理', '云端加速']
})

// 预览用合成模型
const previewModel = computed<AdminModelItem>(() => ({
  id: form.values.id,
  name: form.values.name || '模型名称',
  provider: form.values.provider || '供应商',
  description: form.values.description || '暂无描述。保存后将在前台模型库中完整展示。',
  descriptionEnriched: form.values.descriptionEnriched ?? false,
  contextOrResolution: form.values.contextOrResolution || '2048×2048',
  capabilities: previewCapabilities.value,
  hosting: form.values.hosting,
  termsUrl: '',
  docsUrl: '',
  customPath: form.values.customPath ?? '',
  customParams: form.values.customParams ?? '',
  inputCostPerToken: form.values.inputCostPerToken,
  outputCostPerToken: form.values.outputCostPerToken,
  cacheReadCostPerToken: form.values.cacheReadCostPerToken,
  cacheCreateCostPerToken: form.values.cacheCreateCostPerToken,
  supportsPromptCaching: form.values.supportsPromptCaching,
}))

// 路由透视
const gatewayRoutePreview = ref<GatewayRoutePreviewState>({
  providerSlug: '未指定',
  isFirstParty: false,
  targetPath: '-',
  protocol: '-',
})

async function refreshGatewayRoutePreview() {
  const modelId = form.values.id.trim()
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
      customPath: form.values.customPath?.trim() || undefined,
      customParams: form.values.customParams?.trim() || undefined,
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
  [() => form.values.id, () => form.values.customPath, () => form.values.customParams],
  () => {
    void refreshGatewayRoutePreview()
  },
  { debounce: 300, immediate: true },
)

watch(
  () => props.visible,
  (val) => {
    if (val) {
      form.clearErrors()
      void loadProviderOptions()
      if (props.model) {
        form.resetForm({ ...props.model })
        capabilitiesDraft.value = props.model.capabilities.join(', ')
        showAdvanced.value = Boolean(props.model.customPath || props.model.customParams)
      } else {
        form.resetForm({ ...blankModelForm() })
        capabilitiesDraft.value = ''
        showAdvanced.value = false
      }
    }
  },
  { immediate: true },
)

const handleSubmit = form.handleSubmit((values) => {
  emit(
    'submit',
    finalizeModelSubmit(
      { ...values, descriptionEnriched: false },
      splitCapabilities(capabilitiesDraft.value),
    ),
  )
})
</script>
