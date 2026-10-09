<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.outbound.icon"
    :title="SETTINGS_SECTIONS.outbound.title"
    :description="SETTINGS_MESSAGES.outboundDescription"
    :status="props.plain ? undefined : statusLabel"
    :status-severity="props.plain ? undefined : statusSeverity"
    :plain="props.plain"
    :class="props.class"
  >
    <template #default>
      <div class="flex flex-col gap-5">
        <AdminAlert v-if="error" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ SETTINGS_MESSAGES.outboundLoadFailed }}</span>
            <ActionButton variant="outline" @click="refetch()">
              {{ SETTINGS_MESSAGES.retry }}
            </ActionButton>
          </div>
        </AdminAlert>
        <AdminAlert v-else-if="loading && !setting" severity="secondary">
          {{ SETTINGS_MESSAGES.outboundLoading }}
        </AdminAlert>

        <AdminFormField
          v-slot="{ describedbyId }"
          :label="SETTINGS_MESSAGES.outboundHostsLabel"
          for="outbound-policy-allow-hosts"
          :hint="SETTINGS_MESSAGES.outboundHostsHint"
        >
          <AdminTextarea
            id="outbound-policy-allow-hosts"
            v-model="form.allow_hosts"
            :rows="5"
            class="w-full"
            :disabled="controlsDisabled"
            :aria-describedby="describedbyId"
          />
        </AdminFormField>

        <AdminAlert v-if="validationError" severity="error">
          {{ validationError }}
        </AdminAlert>
      </div>
    </template>

    <template #actions>
      <AdminEditorActions
        :save-disabled="controlsDisabled"
        :reset-disabled="controlsDisabled"
        :saving="saving"
        @save="save"
        @reset="reset"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminEditorActions from '../../shared/components/AdminEditorActions.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminTextarea from '../../shared/components/AdminTextarea.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import SettingEditorCard from '../shared/SettingEditorCard.vue'
import { SETTINGS_SECTIONS } from '../shared/sections'
import { SETTINGS_MESSAGES } from '../pages/messages'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { useOutboundPolicy } from './useOutboundPolicy'
import { useAdminOutboundPolicyFeedback } from '../shared/useSettingsFeedback'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    plain?: boolean
  }>(),
  { class: undefined, plain: false },
)

const feedback = useAdminOutboundPolicyFeedback()
const confirm = useAdminConfirm()
const { error, form, loading, refetch, reset, save, saving, setting, validationError } =
  useOutboundPolicy({ feedback, confirm })

const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
const statusLabel = computed(() => {
  if (error.value) return SETTINGS_MESSAGES.settingsLoadFailed
  if (!setting.value) return SETTINGS_MESSAGES.settingsLoading
  const count = setting.value.allow_hosts.length
  return count ? SETTINGS_MESSAGES.outboundExtraAllowed(count) : SETTINGS_MESSAGES.outboundNoneExtra
})
const statusSeverity = computed(() => {
  if (error.value) return 'danger'
  if (!setting.value) return 'secondary'
  return setting.value.allow_hosts.length ? 'warn' : 'secondary'
})
</script>
