<template>
  <AdminDialogShell
    :visible="visible"
    :header="MODELS_MESSAGES.editHeader(isEditing)"
    :description="MODELS_MESSAGES.editDescription"
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
        <div v-if="firstValidationError" class="text-xs text-destructive" role="alert">
          {{ firstValidationError }}
        </div>
      </div>

      <!-- 右侧：实时卡片预览与网关透视区 (5 列) -->
      <div class="flex flex-col gap-4 lg:col-span-5">
        <CatalogPreviewCard
          :model="previewModel"
          :model-id="form.values.id"
          :route-preview="gatewayRoutePreview"
        />
        <!-- models.dev 参照价：只读透传，不进目录保存 payload -->
        <CatalogReferencePanel
          :model-id="form.values.id"
          :provider="form.values.provider"
          :model-name="form.values.name"
          :enabled="visible"
        />
      </div>
    </div>

    <!-- 弹窗底栏 -->
    <template #footer>
      <AdminDialogActions
        :confirm-label="MODELS_MESSAGES.editSubmit(isEditing)"
        :loading="pending"
        :cancel-disabled="pending"
        @cancel="emit('update:visible', false)"
        @confirm="handleSubmit"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { AdminDialogActions } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { createAdminGatewayResource, type GatewayProviderOption } from '../../resource'
import { type AdminModelItem } from '../../core/catalogData'
import {
  blankModelForm,
  finalizeModelSubmit,
  modelFormSchema,
  splitCapabilities,
} from '../../core/modelForm'
import { useForm, firstFormError } from '@/components/ui/form'
import CatalogIdentityPanel from './components/CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './components/CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './components/CatalogGatewayPanel.vue'
import CatalogReferencePanel from './components/CatalogReferencePanel.vue'
import CatalogPreviewCard, {
  type GatewayRoutePreviewState,
} from './components/CatalogPreviewCard.vue'
import { MODELS_MESSAGES } from '../pages/messages'

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
  return list.length > 0
    ? list
    : [MODELS_MESSAGES.editDefaultCapabilityPrimary, MODELS_MESSAGES.editDefaultCapabilityFallback]
})

// 预览用合成模型
const previewModel = computed<AdminModelItem>(() => ({
  id: form.values.id,
  name: form.values.name || MODELS_MESSAGES.editPreviewName,
  provider: form.values.provider || MODELS_MESSAGES.addColumnProvider,
  description: form.values.description || MODELS_MESSAGES.editPreviewDescription,
  descriptionEnriched: form.values.descriptionEnriched ?? false,
  contextOrResolution: form.values.contextOrResolution || '2048×2048',
  capabilities: previewCapabilities.value,
  hosting: form.values.hosting,
  termsUrl: '',
  docsUrl: '',
  customPath: form.values.customPath ?? '',
  customParams: form.values.customParams ?? '',
  supportsPromptCaching: form.values.supportsPromptCaching,
}))

// 路由透视
const gatewayRoutePreview = ref<GatewayRoutePreviewState>({
  providerSlug: MODELS_MESSAGES.editRouteUnspecified,
  isFirstParty: false,
  targetPath: '-',
  protocol: '-',
})
let routePreviewVersion = 0

async function refreshGatewayRoutePreview() {
  const version = ++routePreviewVersion
  const modelId = form.values.id.trim()
  if (!modelId) {
    gatewayRoutePreview.value = {
      providerSlug: MODELS_MESSAGES.editRouteUnspecified,
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
    if (version !== routePreviewVersion) return
    gatewayRoutePreview.value = {
      providerSlug: preview.provider_slug || MODELS_MESSAGES.editRouteUnspecified,
      isFirstParty: preview.is_first_party,
      targetPath: preview.target_path,
      protocol: preview.path_source_label || preview.path_source,
    }
  } catch {
    if (version !== routePreviewVersion) return
    gatewayRoutePreview.value = {
      providerSlug: MODELS_MESSAGES.editRouteUnspecified,
      isFirstParty: false,
      targetPath: '-',
      protocol: MODELS_MESSAGES.editRouteUnavailable,
    }
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
