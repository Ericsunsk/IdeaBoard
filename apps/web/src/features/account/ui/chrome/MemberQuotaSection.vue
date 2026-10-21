<template>
  <section class="flex min-w-0 flex-col gap-3" :aria-label="t('workspace.quota.title')">
    <!-- Self usage: visible to every member -->
    <div
      v-if="section.isSelfLoading.value"
      class="flex min-h-24 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Spinner class="size-6 text-primary" />
      <span class="text-xs text-muted-foreground">
        {{ t('common.actions.loading') }}
      </span>
    </div>

    <LoadErrorPanel
      v-else-if="section.hasSelfError.value"
      :message="t('workspace.quota.loadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retrySelf()"
    />

    <div v-else-if="section.selfQuota.value" :class="accountCardPanelClass">
      <div class="flex items-center justify-between gap-2">
        <h3 class="m-0 text-admin-body font-semibold text-foreground">
          {{ t('workspace.quota.title') }}
        </h3>
        <span class="text-xs text-muted-foreground">
          {{ periodLabel(section.selfQuota.value.period) }}
        </span>
      </div>

      <p class="m-0 mt-2 text-sm text-foreground" aria-live="polite">
        <template v-if="section.selfQuota.value.limitCredits != null">
          {{
            t('workspace.quota.usedOf', {
              used: section.selfQuota.value.usedCredits,
              limit: section.selfQuota.value.limitCredits,
            })
          }}
        </template>
        <template v-else>
          {{ t('workspace.quota.usedUnlimited', { used: section.selfQuota.value.usedCredits }) }}
        </template>
      </p>
      <p
        v-if="section.selfQuota.value.remainingCredits != null"
        class="m-0 mt-0.5 text-xs text-muted-foreground"
      >
        {{
          t('workspace.quota.remaining', { remaining: section.selfQuota.value.remainingCredits })
        }}
      </p>
      <p
        v-if="section.selfQuota.value.periodStart && section.selfQuota.value.periodEnd"
        class="m-0 mt-0.5 text-xs text-muted-foreground"
      >
        {{
          t('workspace.quota.periodRange', {
            start: section.selfQuota.value.periodStart,
            end: section.selfQuota.value.periodEnd,
          })
        }}
      </p>
      <p
        v-if="section.isSelfExhausted.value"
        class="m-0 mt-2 text-xs leading-normal text-destructive"
        role="alert"
      >
        {{ t('workspace.quota.exhausted') }}
      </p>
    </div>

    <!-- Management controls (billing managers only) -->
    <div v-if="section.canManage.value" :class="accountCardPanelClass">
      <div
        v-if="section.isListLoading.value"
        class="flex min-h-16 flex-col items-center justify-center gap-2"
        aria-live="polite"
      >
        <Spinner class="size-5 text-primary" />
      </div>

      <LoadErrorPanel
        v-else-if="section.hasListError.value"
        :message="t('workspace.quota.loadFailed')"
        :retry-label="t('common.actions.retry')"
        @retry="section.retryList()"
      />

      <div v-else class="flex flex-col gap-2">
        <p v-if="section.quotas.value.length === 0" class="m-0 text-xs text-muted-foreground">
          {{ t('workspace.quota.noQuotas') }}
        </p>
        <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
          <li
            v-for="row in section.quotas.value"
            :key="row.userId"
            class="flex items-center justify-between gap-2"
          >
            <div class="min-w-0">
              <p class="m-0 truncate text-xs font-medium text-foreground" :title="row.userId">
                {{ section.memberLabel(row.userId) }}
              </p>
              <p class="m-0 mt-0.5 text-xs text-muted-foreground">
                <span>
                  <template v-if="row.limitCredits != null">
                    {{
                      t('workspace.quota.usedOf', {
                        used: row.usedCredits,
                        limit: row.limitCredits,
                      })
                    }}
                  </template>
                  <template v-else>
                    {{ t('workspace.quota.usedUnlimited', { used: row.usedCredits }) }}
                  </template>
                </span>
                <span>{{ periodLabel(row.period) }}</span>
              </p>
            </div>
            <Button
              v-if="row.period !== 'unlimited'"
              type="button"
              variant="outline"
              size="sm"
              class="h-8 shrink-0 px-3"
              :is-disabled="section.isSaving.value"
              @click="section.clearQuota(row.userId)"
            >
              {{ t('workspace.quota.setUnlimited') }}
            </Button>
          </li>
        </ul>

        <div class="mt-1 flex flex-col gap-2">
          <label :for="memberSelectId" class="text-xs font-medium text-foreground">
            {{ t('workspace.quota.memberLabel') }}
          </label>
          <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
          <Select
            :model-value="section.targetUserId.value"
            :disabled="section.isSaving.value"
            @update:model-value="section.pickTargetUser($event)"
          >
            <SelectTrigger
              :id="memberSelectId"
              size="sm"
              class="w-full cursor-pointer gap-1.5"
              :aria-label="t('workspace.quota.memberLabel')"
            >
              <SelectValue :placeholder="t('workspace.quota.pickMember')">
                {{ selectedMemberLabel }}
              </SelectValue>
              <SelectIcon>
                <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
              </SelectIcon>
            </SelectTrigger>
            <SelectContent position="popper" align="start" :side-offset="appOverlayMenuSideOffset">
              <SelectViewport>
                <SelectItem
                  v-for="userId in section.memberOptions.value"
                  :key="userId"
                  :value="userId"
                >
                  <SelectItemText>{{ section.memberLabel(userId) }}</SelectItemText>
                </SelectItem>
              </SelectViewport>
            </SelectContent>
          </Select>

          <label :for="periodSelectId" class="text-xs font-medium text-foreground">
            {{ t('workspace.quota.periodLabel') }}
          </label>
          <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
          <Select
            :model-value="section.targetPeriod.value"
            :disabled="section.isSaving.value"
            @update:model-value="section.pickTargetPeriod($event)"
          >
            <SelectTrigger
              :id="periodSelectId"
              size="sm"
              class="w-full cursor-pointer gap-1.5"
              :aria-label="t('workspace.quota.periodLabel')"
            >
              <SelectValue>{{ periodLabel(section.targetPeriod.value) }}</SelectValue>
              <SelectIcon>
                <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
              </SelectIcon>
            </SelectTrigger>
            <SelectContent position="popper" align="start" :side-offset="appOverlayMenuSideOffset">
              <SelectViewport>
                <SelectItem value="daily">
                  <SelectItemText>{{ t('workspace.quota.periodDaily') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="weekly">
                  <SelectItemText>{{ t('workspace.quota.periodWeekly') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="monthly">
                  <SelectItemText>{{ t('workspace.quota.periodMonthly') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="fixed">
                  <SelectItemText>{{ t('workspace.quota.periodFixed') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="unlimited">
                  <SelectItemText>{{ t('workspace.quota.periodUnlimited') }}</SelectItemText>
                </SelectItem>
              </SelectViewport>
            </SelectContent>
          </Select>

          <template v-if="section.targetPeriod.value !== 'unlimited'">
            <label :for="limitInputId" class="text-xs font-medium text-foreground">
              {{ t('workspace.quota.limitLabel') }}
            </label>
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The limit input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
            <Input
              :id="limitInputId"
              v-model="section.limitDraft.value"
              size="sm"
              class="h-9 min-w-0 flex-1 bg-background text-xs"
              :disabled="section.isSaving.value"
              :aria-label="t('workspace.quota.limitLabel')"
              inputmode="numeric"
              @keydown.enter="section.saveQuota()"
            />
          </template>
          <div class="flex shrink-0 gap-2">
            <Button
              type="button"
              size="sm"
              class="h-9 px-4"
              :is-loading="section.isSaving.value"
              :is-disabled="section.isSaving.value"
              @click="section.saveQuota()"
            >
              {{ section.isSaving.value ? t('workspace.quota.saving') : t('workspace.quota.save') }}
            </Button>
          </div>
        </div>
        <p
          v-if="section.saveError.value"
          class="m-0 text-xs leading-normal text-destructive"
          role="alert"
        >
          {{ section.saveError.value }}
        </p>
      </div>
    </div>
    <p v-else class="m-0 text-xs text-muted-foreground">
      {{ t('workspace.quota.managerOnly') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NavArrowDown } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { appOverlayMenuSideOffset } from '../../../../theme/appOverlayMenu'
import { useMemberQuotaSection } from '../../session/quota/useMemberQuotaSection'
import { quotaPeriodLabelKey, type MemberQuotaPeriod } from '../../session/quota/resource'
import { accountCardPanelClass } from '../shared/ui'

const props = withDefaults(
  defineProps<{
    workspaceId: string
    consumptionScope?: string | null | undefined
    isExpanded?: boolean | undefined
  }>(),
  {
    consumptionScope: null,
    isExpanded: true,
  },
)

const { t } = useI18n()

const section = useMemberQuotaSection({
  workspaceId: () => props.workspaceId,
  consumptionScope: () => props.consumptionScope,
  isExpanded: () => props.isExpanded,
})

const memberSelectId = `member-quota-member-${props.workspaceId}`
const periodSelectId = `member-quota-period-${props.workspaceId}`
const limitInputId = `member-quota-limit-${props.workspaceId}`

function periodLabel(period: MemberQuotaPeriod): string {
  return t(quotaPeriodLabelKey(period))
}

const selectedMemberLabel = computed(() => {
  const userId = section.targetUserId.value.trim()
  if (!userId) return ''
  return section.memberLabel(userId)
})
</script>
