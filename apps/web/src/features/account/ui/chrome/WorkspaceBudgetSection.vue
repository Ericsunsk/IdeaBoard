<template>
  <section class="flex min-w-0 flex-col gap-3" :aria-label="t('workspace.budget.title')">
    <!-- Loading State -->
    <div
      v-if="section.isLoading.value"
      class="flex min-h-24 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Spinner class="size-6 text-primary" />
      <span class="text-xs text-muted-foreground">
        {{ t('common.actions.loading') }}
      </span>
    </div>

    <!-- Error State -->
    <LoadErrorPanel
      v-else-if="section.hasLoadError.value"
      :message="t('workspace.budget.loadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retryBudget()"
    />

    <!-- Loaded Content -->
    <div v-else-if="section.budget.value" :class="accountCardPanelClass">
      <div class="flex items-center justify-between gap-2">
        <h3 class="m-0 text-admin-body font-semibold text-foreground">
          {{ t('workspace.budget.title') }}
        </h3>
        <span class="text-xs text-muted-foreground">
          {{
            section.budget.value.period === 'monthly'
              ? t('workspace.budget.periodMonthly')
              : t('workspace.budget.periodUnlimited')
          }}
        </span>
      </div>

      <p class="m-0 mt-2 text-sm text-foreground" aria-live="polite">
        <template v-if="section.budget.value.limitCredits != null">
          {{
            t('workspace.budget.usedOf', {
              used: section.budget.value.usedCredits,
              limit: section.budget.value.limitCredits,
            })
          }}
        </template>
        <template v-else>
          {{ t('workspace.budget.usedUnlimited', { used: section.budget.value.usedCredits }) }}
        </template>
      </p>
      <p
        v-if="section.budget.value.remainingCredits != null"
        class="m-0 mt-0.5 text-xs text-muted-foreground"
      >
        {{ t('workspace.budget.remaining', { remaining: section.budget.value.remainingCredits }) }}
      </p>
      <p
        v-if="section.budget.value.periodStart && section.budget.value.periodEnd"
        class="m-0 mt-0.5 text-xs text-muted-foreground"
      >
        {{
          t('workspace.budget.periodRange', {
            start: section.budget.value.periodStart,
            end: section.budget.value.periodEnd,
          })
        }}
      </p>
      <p
        v-if="section.isExhausted.value"
        class="m-0 mt-2 text-xs leading-normal text-destructive"
        role="alert"
      >
        {{ t('workspace.budget.exhausted') }}
      </p>

      <!-- Management controls (billing managers only) -->
      <div v-if="section.canManage.value" class="mt-3 flex flex-col gap-2">
        <label :for="limitInputId" class="text-xs font-medium text-foreground">
          {{ t('workspace.budget.limitLabel') }}
        </label>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
          <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The limit input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
          <Input
            :id="limitInputId"
            v-model="section.limitDraft.value"
            size="sm"
            class="h-9 min-w-0 flex-1 bg-background text-xs"
            :disabled="section.isSaving.value"
            :aria-label="t('workspace.budget.limitLabel')"
            inputmode="numeric"
            @keydown.enter="section.saveMonthly()"
          />
          <div class="flex shrink-0 gap-2">
            <Button
              type="button"
              size="sm"
              class="h-9 px-4"
              :is-loading="section.isSaving.value"
              :is-disabled="section.isSaving.value"
              @click="section.saveMonthly()"
            >
              {{
                section.isSaving.value ? t('workspace.budget.saving') : t('workspace.budget.save')
              }}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-9 px-4"
              :is-disabled="section.isSaving.value"
              @click="section.saveUnlimited()"
            >
              {{ t('workspace.budget.setUnlimited') }}
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
      <p v-else class="m-0 mt-2 text-xs text-muted-foreground">
        {{ t('workspace.budget.managerOnly') }}
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { useWorkspaceBudgetSection } from '../../session/budget/useWorkspaceBudgetSection'
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

const section = useWorkspaceBudgetSection({
  workspaceId: () => props.workspaceId,
  consumptionScope: () => props.consumptionScope,
  isExpanded: () => props.isExpanded,
})

const limitInputId = `workspace-budget-limit-${props.workspaceId}`

watch(
  () => [props.workspaceId, section.budget.value?.period, section.budget.value?.limitCredits],
  () => section.startEditing(),
  { immediate: true },
)
</script>
