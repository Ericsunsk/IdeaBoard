<template>
  <AdminDialogShell
    v-model:visible="editor.visible.value"
    :header="editor.isEditing.value ? '编辑供应商端点' : '新增端点'"
    :busy="editor.saveLoading.value"
  >
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="端点名称"
      for="admin-provider-endpoint-name"
      :error="editor.errors.value.name"
    >
      <InputText
        id="admin-provider-endpoint-name"
        v-model="editor.form.value.name"
        class="w-full"
        placeholder="例如：Nano Banana 官方端点"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="editor.clearError('name')"
      />
    </AdminFormField>

    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="服务协议"
      for="admin-provider-endpoint-protocol"
      :error="editor.errors.value.protocol"
    >
      <Select
        v-model="editor.form.value.protocol"
        input-id="admin-provider-endpoint-protocol"
        aria-label="服务协议"
        class="w-full"
        :options="providerProtocolOptions"
        option-label="label"
        option-value="value"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="handleProtocolChange"
      />
    </AdminFormField>

    <AdminFormField
      v-if="isImagePipelineProtocol(editor.form.value.protocol)"
      v-slot="{ describedbyId, invalid }"
      label="处理功能"
      for="admin-provider-endpoint-capability"
      :error="editor.errors.value.capability"
    >
      <Select
        v-model="editor.form.value.capability"
        input-id="admin-provider-endpoint-capability"
        aria-label="处理功能"
        class="w-full"
        :options="providerProcessingCapabilityOptions"
        option-label="label"
        option-value="value"
        placeholder="选择处理功能"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="editor.clearError('capability')"
      />
    </AdminFormField>

    <AdminFormField
      v-if="editor.form.value.protocol === 'model-api'"
      v-slot="{ describedbyId, invalid }"
      label="服务类型"
      for="admin-provider-endpoint-type"
      :error="editor.errors.value.apiType"
    >
      <Select
        v-model="editor.form.value.apiType"
        input-id="admin-provider-endpoint-type"
        aria-label="服务类型"
        class="w-full"
        :options="editor.apiTypeOptions"
        option-label="label"
        option-value="value"
        placeholder="选择服务类型"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="editor.clearError('apiType')"
      />
    </AdminFormField>

    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="Base URL地址"
      for="admin-provider-endpoint-api-url"
      :error="editor.errors.value.baseUrl"
    >
      <InputText
        id="admin-provider-endpoint-api-url"
        v-model="editor.form.value.baseUrl"
        class="w-full"
        placeholder="例如：https://api.example.com/v1"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @blur="editor.handleApiUrlBlur"
        @update:model-value="editor.clearError('baseUrl')"
      />
    </AdminFormField>

    <AdminFormField
      v-if="isModellessProviderProtocol(editor.form.value.protocol)"
      v-slot="{ describedbyId, invalid }"
      label="API Key"
      for="admin-provider-endpoint-api-key"
      :error="editor.errors.value.apiKey"
    >
      <InputText
        id="admin-provider-endpoint-api-key"
        v-model="editor.form.value.apiKey"
        type="password"
        class="w-full"
        autocomplete="new-password"
        placeholder="输入图片处理 API Key"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="editor.clearError('apiKey')"
      />
    </AdminFormField>

    <template #footer>
      <Button
        label="取消"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :disabled="editor.saveLoading.value"
        @click="editor.close"
      />
      <Button
        label="确定"
        :class="adminActionButtonClass"
        severity="contrast"
        :loading="editor.saveLoading.value"
        @click="editor.save"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { useAdminProviderConfigContext } from './useProviderConfig'
import {
  getAdminProviderEndpointProtocolLabel,
  isImageFillProtocol,
  isImagePipelineProtocol,
  isModellessProviderProtocol,
} from './payload'

const { providerEndpointEditor: editor } = useAdminProviderConfigContext()

const providerProtocolOptions = computed(() =>
  editor.protocolRegistry.value.map((protocol) => ({
    label: getAdminProviderEndpointProtocolLabel(protocol.id),
    value: protocol.id,
  })),
)

const providerProcessingCapabilityOptions = computed(() => {
  const protocol = editor.protocolRegistry.value.find(
    (item) => item.id === editor.form.value.protocol,
  )
  return (protocol?.capabilities ?? []).map((capability) => ({
    label: capability,
    value: capability,
  }))
})

function handleProtocolChange(protocol: string) {
  if (isImageFillProtocol(protocol)) {
    editor.form.value.apiType = 'self-hosted'
    editor.form.value.capability = 'fill'
  } else if (isImagePipelineProtocol(protocol)) {
    editor.form.value.apiType = 'self-hosted'
    editor.form.value.capability = ''
  } else {
    editor.form.value.apiKey = ''
    editor.form.value.capability = ''
  }
  editor.clearError('protocol')
  editor.clearError('apiKey')
  editor.clearError('capability')
}
</script>
