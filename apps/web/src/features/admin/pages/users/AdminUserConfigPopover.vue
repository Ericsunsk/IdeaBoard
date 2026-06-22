<template>
  <Popover ref="popoverRef" :pt="userConfigPopoverPt">
    <div :class="adminConfigPopoverClass">
      <div :class="adminPopoverSectionHeaderClass">
        <div :class="adminPopoverTitleClass">用户配置</div>
      </div>

      <div
        class="flex items-center justify-between rounded-lg border border-admin-border bg-admin-surface-muted px-3 py-2"
      >
        <span class="text-admin-caption font-bold text-admin-text-2">开放注册</span>
        <ToggleSwitch
          :model-value="registrationEnabled"
          size="small"
          :disabled="registrationSettingLoading"
          aria-label="开放注册"
          @update:model-value="handleRegistrationUpdate"
        />
      </div>

      <div class="flex flex-col gap-2 rounded-lg border border-admin-border bg-white p-3">
        <div class="flex items-center gap-1.5 text-admin-meta font-bold text-admin-text-2">
          <Settings class="size-4 text-admin-text-3" />
          <span>限流策略</span>
        </div>
        <div class="mt-1 grid grid-cols-1 gap-2.5">
          <label class="flex min-w-0 flex-col gap-1 text-admin-meta font-[650] text-admin-text-2">
            <span>最大请求 (次)</span>
            <InputNumber
              :model-value="rateLimitForm.max_requests"
              :min="1"
              :max="maxRequestsLimit"
              placeholder="100"
              size="small"
              fluid
              @update:model-value="updateRateLimitField('max_requests', $event)"
            />
          </label>
          <label class="flex min-w-0 flex-col gap-1 text-admin-meta font-[650] text-admin-text-2">
            <span>窗口周期 (秒)</span>
            <InputNumber
              :model-value="rateLimitForm.window_seconds"
              :min="1"
              :max="windowSecondsLimit"
              placeholder="60"
              size="small"
              fluid
              @update:model-value="updateRateLimitField('window_seconds', $event)"
            />
          </label>
        </div>
        <Button
          label="保存配置"
          size="small"
          class="mt-2"
          :loading="rateLimitSaving"
          fluid
          @click="$emit('save-rate-limit')"
        />
      </div>
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Settings } from '@iconoir/vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Popover from 'primevue/popover'
import ToggleSwitch from 'primevue/toggleswitch'
import { createAdminPopoverPt } from '../../shared/lib/adminOverlayPt'
import {
  adminConfigPopoverClass,
  adminPopoverSectionHeaderClass,
  adminPopoverTitleClass,
} from '../../shared/lib/adminUiClasses'

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

type AdminUserRateLimitForm = {
  window_seconds: number | null
  max_requests: number | null
}

const props = withDefaults(
  defineProps<{
    registrationEnabled: boolean
    registrationSettingLoading?: boolean
    rateLimitForm: AdminUserRateLimitForm
    rateLimitSaving?: boolean
    maxRequestsLimit?: number
    windowSecondsLimit?: number
  }>(),
  {
    registrationSettingLoading: false,
    rateLimitSaving: false,
    maxRequestsLimit: 10_000,
    windowSecondsLimit: 3_600,
  },
)

const emit = defineEmits<{
  'update:registrationEnabled': [value: boolean]
  'toggle-registration': [value: boolean]
  'update:rateLimitForm': [value: AdminUserRateLimitForm]
  'save-rate-limit': []
}>()

const popoverRef = ref<PrimePopupPanel | null>(null)
const userConfigPopoverPt = createAdminPopoverPt({ contentClass: '!p-0' })

function toggle(event: Event) {
  popoverRef.value?.toggle(event)
}

function hide() {
  popoverRef.value?.hide?.()
}

function handleRegistrationUpdate(value: boolean) {
  emit('update:registrationEnabled', value)
  emit('toggle-registration', value)
}

function updateRateLimitField(field: keyof AdminUserRateLimitForm, value: number | null) {
  emit('update:rateLimitForm', {
    ...props.rateLimitForm,
    [field]: value,
  })
}

defineExpose({
  toggle,
  hide,
})
</script>
